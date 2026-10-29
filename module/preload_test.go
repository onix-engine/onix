package module

import (
	"context"
	"errors"
	"image"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/render"
)

type preloadFeature struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	err     error
}

func (f *preloadFeature) ConfigureResources(c project.ResourceLoadContext, _ project.ResourceEntries) error {
	c.SetImageDecoder(f)
	return nil
}

func (f *preloadFeature) DecodeImage(_ []byte, _ string, options graphics.TextureOptions) (*graphics.Texture, error) {
	f.once.Do(func() { close(f.started) })
	<-f.release
	if f.err != nil {
		return nil, f.err
	}
	return graphics.TextureFromOwnedNRGBA(image.NewNRGBA(image.Rect(0, 0, 2, 2)), options), nil
}

type preloadDriver struct {
	render.Driver
	uploads atomic.Uint32
	err     error
}

func (d *preloadDriver) CreateTexture(render.TextureDescriptor, []byte) (render.TextureHandle, error) {
	if d.err != nil {
		return render.TextureHandle{}, d.err
	}
	return render.NewHandle[render.TextureResource](d.uploads.Add(1), 1), nil
}
func (*preloadDriver) DestroyTexture(render.TextureHandle) error { return nil }
func (*preloadDriver) Destroy()                                  {}

func asyncPreloadFixture(t *testing.T, loadErr error) (*Runtime, *Scene, *ScenePreloader, *preloadDriver, func()) {
	t.Helper()
	plan, source := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json","next.json"]}`,
		"resources/scenes/main.json": `{"modules":[{"id":"base","mod":"base"}]}`,
		"resources/scenes/next.json": `{"modules":[{"id":"a","mod":"images"},{"id":"b","mod":"images"}]}`,
		"base/mod.json":              `{}`,
		"images/mod.json":            `{"components":{"sprite":{"texture":{"image":"a.png"}}},"elements":[{"id":"other","components":{"sprite":{"texture":{"image":"b.png"}}}}]}`,
		"images/a.png":               "decoded by injected feature",
		"images/b.png":               "second image decoded by injected feature",
	})
	driver := &preloadDriver{}
	device, _ := render.NewDevice(driver)
	t.Cleanup(device.Destroy)
	device.Activate()
	feature := &preloadFeature{started: make(chan struct{}), release: make(chan struct{}), err: loadErr}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(feature.release) }) }
	t.Cleanup(release)
	r := newFixtureWith[Render2D](t, plan, Registry{}, Options{Source: source, Resources: feature})
	graphics.BindFrame(r.world, graphics.NewCanvas2D(device, 320, 200))
	main, err := r.Manager().StartEntry()
	must(t, err)
	p, err := r.Manager().BeginPreload(ScenePreloadRequest{Scene: "next.json"})
	must(t, err)
	t.Cleanup(p.Discard)
	select {
	case <-feature.started:
	case <-time.After(time.Second):
		t.Fatal("background decoder did not start")
	}
	return r, main, p, driver, release
}

func waitPreloadCPU(t *testing.T, p *ScenePreloader) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	must(t, p.cpu.WaitCPU(ctx))
}

func TestAsyncScenePreloadKeepsForegroundAndBudgetsUploads(t *testing.T) {
	r, main, p, driver, release := asyncPreloadFixture(t, nil)
	count := r.world.EntityCount()
	if !errors.Is(main.context.Switch(SceneSwitchRequest{Scene: "next.json"}), ErrBusy) {
		t.Fatal("scene callback bypassed pending preload ownership")
	}
	if _, err := r.Mount("next.json"); !errors.Is(err, ErrBusy) {
		t.Fatal("pending scene was mounted a second time", err)
	}
	if ready, err := p.Advance(time.Second); ready || err != nil || r.world.EntityCount() != count {
		t.Fatal("unfinished CPU work blocked Advance or modified ECS", err)
	}
	must(t, r.Dispatch())
	if r.Manager().Active() != main || main.State() != SceneActive {
		t.Fatal("worker changed foreground scene")
	}
	release()
	waitPreloadCPU(t, p)
	if driver.uploads.Load() != 0 || r.world.EntityCount() != count {
		t.Fatal("CPU worker accessed ECS or the active render device")
	}
	if ready, err := p.Advance(0); ready || err != nil || !p.CPUReady() || p.Ready() {
		t.Fatal("CPU and GPU readiness were conflated", err)
	}
	if err := r.Manager().Activate(p.Scene(), SceneActivationOptions{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("scene activated before upload", err)
	}
	device := render.ActiveDevice()
	if ready, err := p.AdvanceFor(device, graphics.UploadBudget{Time: time.Second, Resources: 1}); ready || err != nil || driver.uploads.Load() != 1 {
		t.Fatal("upload ignored resource budget", err)
	}
	if ready, err := p.Advance(0); !ready || err != nil || driver.uploads.Load() != 2 || !p.GPUReadyFor(device) {
		t.Fatal("deduplicated uploads failed to complete", err)
	}
	other, _ := render.NewDevice(&preloadDriver{})
	other.Activate()
	if !p.Ready() {
		t.Fatal("global device changed owning-world readiness")
	}
	graphics.BindFrame(r.world, graphics.NewCanvas2D(other, 320, 200))
	if p.Ready() || !errors.Is(r.Manager().Activate(p.Scene(), SceneActivationOptions{}), ErrUnavailable) {
		t.Fatal("foreign device accepted an unuploaded scene")
	}
	other.Destroy()
	device.Activate()
	graphics.BindFrame(r.world, graphics.NewCanvas2D(device, 320, 200))
	must(t, r.Manager().Activate(p.Scene(), SceneActivationOptions{}))
	must(t, r.Dispatch())
	if r.Manager().Active() != p.Scene() || p.Scene().State() != SceneActive {
		t.Fatal("activation did not use the prepared scene")
	}
	if ready, err := p.Advance(0); !ready || err != nil || p.Scene().State() != SceneActive {
		t.Fatal("repeated Advance changed an active scene's state", err)
	}
	p.Discard()
	if !p.Scene().Alive() {
		t.Fatal("discard destroyed the committed foreground")
	}
}

func TestAsyncScenePreloadCancelAndFailurePreserveForeground(t *testing.T) {
	for _, cancelled := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel", false: "decode-error"}[cancelled], func(t *testing.T) {
			var cause error
			if !cancelled {
				cause = errors.New("bad image")
			}
			r, main, p, driver, release := asyncPreloadFixture(t, cause)
			count, stores := r.world.EntityCount(), len(r.contentResources)
			if cancelled {
				p.Discard()
			}
			release()
			waitPreloadCPU(t, p)
			if !cancelled {
				if ready, err := p.Advance(0); ready || !errors.Is(err, cause) {
					t.Fatal("worker failure was not reported", err)
				}
			}
			if r.Manager().Active() != main || !main.Alive() || r.world.EntityCount() != count || len(r.contentResources) != stores || driver.uploads.Load() != 0 || r.manager.preload != nil {
				t.Fatal("failed or cancelled preparation changed foreground ownership")
			}
		})
	}
}

func TestAsyncScenePreloadUploadFailureReleasesHiddenScene(t *testing.T) {
	r, main, p, driver, release := asyncPreloadFixture(t, nil)
	count, stores := r.world.EntityCount(), len(r.contentResources)
	release()
	waitPreloadCPU(t, p)
	if ready, err := p.Advance(0); ready || err != nil || !p.CPUReady() {
		t.Fatal("CPU preparation failed", err)
	}
	cause := errors.New("device upload failed")
	driver.err = cause
	if ready, err := p.Advance(0); ready || !errors.Is(err, cause) {
		t.Fatal("GPU error was not reported", err)
	}
	if r.Manager().Active() != main || !main.Alive() || p.Scene().State() != SceneDisposed || r.world.EntityCount() != count || len(r.contentResources) != stores || p.Scene().uploads.Remaining() != 0 {
		t.Fatal("GPU failure leaked the hidden scene or changed the foreground")
	}
}

func TestBeginPreloadRejectsQueuedTransition(t *testing.T) {
	plan, registry, _ := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	_, err := r.Manager().StartEntry()
	must(t, err)
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json"}))
	if _, err := r.Manager().BeginPreload(ScenePreloadRequest{Scene: "broken.json"}); !errors.Is(err, ErrBusy) {
		t.Fatal("preload started alongside a queued transition", err)
	}
}
