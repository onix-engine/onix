package project

import (
	"context"
	"image"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
)

func TestScenePreloaderActivatesPreparedScene(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"loading","scenes":[{"id":"loading","path":"scenes/loading.scene.json"},{"id":"menu","path":"scenes/menu.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/loading.scene.json", `{"nodes":[{"id":"loading","coordinateSpace":"ui"}]}`)
	writeTestFile(t, dir, "resources/scenes/menu.scene.json", `{"nodes":[{"id":"menu","coordinateSpace":"ui"},{"id":"caption","coordinateSpace":"ui","components":{"text":{"value":"Ready"}}}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()

	world := ecs.NewWorld()
	active, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	preloader, err := bundle.BeginPreloadScene(world, "menu")
	if err != nil {
		t.Fatal(err)
	}
	for !preloader.Ready() {
		if _, err := preloader.Advance(0); err != nil {
			t.Fatal(err)
		}
	}
	preparedCaption, err := preloader.scene.Entity("caption")
	if err != nil {
		t.Fatal(err)
	}
	preparedVisibility, ok := ecs.Get[scene.Visibility](world, preparedCaption)
	if !ok || preparedVisibility.Visible {
		t.Fatalf("prepared caption visibility = %#v, want hidden", preparedVisibility)
	}
	if scene.IsActive(world, preparedCaption) {
		t.Fatal("preloaded scene was allowed to simulate")
	}
	if err := preloader.Activate(active); err != nil {
		t.Fatal(err)
	}
	defer active.Dispose()
	if active.SceneID() != "menu" {
		t.Fatalf("active scene = %q, want menu", active.SceneID())
	}
	if _, err := active.Entity("loading"); err == nil {
		t.Fatal("loading scene entity remained after activation")
	}
	caption, err := active.Entity("caption")
	if err != nil {
		t.Fatal(err)
	}
	visibility, ok := ecs.Get[scene.Visibility](world, caption)
	if !ok || !visibility.Visible {
		t.Fatalf("prepared caption visibility = %#v, want visible", visibility)
	}
	if !scene.IsActive(world, caption) {
		t.Fatal("activated scene remained paused")
	}
}

type gatedSceneDecoder struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (d *gatedSceneDecoder) DecodeImage(_ []byte, _ string, options graphics.TextureOptions) (*graphics.Texture, error) {
	d.once.Do(func() { close(d.started) })
	<-d.release
	return graphics.TextureFromOwnedNRGBA(image.NewNRGBA(image.Rect(0, 0, 2, 2)), options), nil
}

type preloadDriver struct {
	render.Driver
	uploads atomic.Uint32
}

func (d *preloadDriver) CreateTexture(render.TextureDescriptor, []byte) (render.TextureHandle, error) {
	return render.NewHandle[render.TextureResource](d.uploads.Add(1), 1), nil
}
func (*preloadDriver) DestroyTexture(render.TextureHandle) error { return nil }
func (*preloadDriver) Destroy()                                  {}

func TestScenePreloaderSeparatesWorkerAndBudgetedDeviceUpload(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"loading","scenes":[{"id":"loading","path":"scenes/loading.scene.json"},{"id":"menu","path":"scenes/menu.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/loading.scene.json", `{"nodes":[{"id":"loading"}]}`)
	writeTestFile(t, dir, "resources/scenes/menu.scene.json", `{"nodes":[{"id":"first","components":{"sprite":{"texture":{"image":"resources/a.png"}}}},{"id":"second","components":{"sprite":{"texture":{"image":"resources/b.png"}}}}]}`)
	writeTestBinaryFile(t, dir, "resources/a.png", tinyPNG(t))
	writeTestBinaryFile(t, dir, "resources/b.png", append(tinyPNG(t), 0))
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	driver := &preloadDriver{}
	device, _ := render.NewDevice(driver)
	defer device.Destroy()
	device.Activate()
	w := ecs.NewWorld()
	request := &scene.RenderRequest{}
	ecs.InsertResource(w, request)
	active, err := bundle.InstantiateEntryScene(w)
	if err != nil {
		t.Fatal(err)
	}
	defer active.Dispose()
	decoder := &gatedSceneDecoder{started: make(chan struct{}), release: make(chan struct{})}
	bundle.resources.imageDecoder = decoder
	var release sync.Once
	defer release.Do(func() { close(decoder.release) })
	p, err := bundle.BeginPreloadScene(w, "menu")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Discard()
	select {
	case <-decoder.started:
	case <-time.After(time.Second):
		t.Fatal("background decoding did not start")
	}
	count := w.EntityCount()
	if ready, err := p.Advance(time.Second); ready || err != nil || w.EntityCount() != count {
		t.Fatal("blocked decoder blocked Advance or modified ECS", err)
	}
	release.Do(func() { close(decoder.release) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.cpu.WaitCPU(ctx); err != nil {
		t.Fatal(err)
	}
	if !request.Take() || driver.uploads.Load() != 0 || w.EntityCount() != count {
		t.Fatal("worker did not wake, or accessed ECS/device")
	}
	for i := 0; !p.CPUReady() && i < 100; i++ {
		if _, err := p.AdvanceFor(device, graphics.UploadBudget{}); err != nil {
			t.Fatal(err)
		}
	}
	if !p.CPUReady() || p.GPUReadyFor(device) || p.Ready() || driver.uploads.Load() != 0 {
		t.Fatal("CPU readiness was conflated with GPU readiness")
	}
	if err := p.Activate(active); err == nil || active.SceneID() != "loading" {
		t.Fatal("unuploaded scene replaced the foreground")
	}
	if ready, err := p.AdvanceFor(device, graphics.UploadBudget{Time: time.Second, Resources: 1}); ready || err != nil || driver.uploads.Load() != 1 {
		t.Fatal("first upload exceeded resource budget", err)
	}
	if ready, err := p.AdvanceFor(device, graphics.UploadBudget{}); !ready || err != nil || !p.GPUReadyFor(device) {
		t.Fatal("prepared scene did not become GPU ready", err)
	}
	if err := p.Activate(active); err != nil {
		t.Fatal(err)
	}
}
