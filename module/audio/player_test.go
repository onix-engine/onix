package moduleaudio

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
)

type silentPlayback struct {
	playing       bool
	position      time.Duration
	volume        float64
	plays, closes int
}

func (p *silentPlayback) Play() error                    { p.playing = true; p.plays++; return nil }
func (p *silentPlayback) Pause() error                   { p.playing = false; return nil }
func (p *silentPlayback) Stop() error                    { p.playing = false; p.position = 0; return nil }
func (p *silentPlayback) IsPlaying() bool                { return p.playing }
func (p *silentPlayback) Position() time.Duration        { return p.position }
func (p *silentPlayback) Duration() time.Duration        { return time.Minute }
func (p *silentPlayback) Seek(value time.Duration) error { p.position = value; return nil }
func (p *silentPlayback) Volume() float64                { return p.volume }
func (p *silentPlayback) SetVolume(value float64) error  { p.volume = value; return nil }
func (p *silentPlayback) Err() error                     { return nil }
func (p *silentPlayback) Close() error                   { p.closes++; p.playing = false; return nil }

func TestModuleAudioPlayerRetainsCursorAndHonorsPauseIntent(t *testing.T) {
	files := fstest.MapFS{}
	for name, data := range map[string]string{"resources/project.json": `{"entry":"main.json","scenes":["main.json","game.json"]}`, "resources/scenes/main.json": `{"modules":[{"id":"a","mod":"speaker"}]}`, "resources/scenes/game.json": `{"modules":[]}`, "speaker/mod.json": `{}`} {
		files[name] = &fstest.MapFile{Data: []byte(data)}
	}
	plan, err := project.CompileProject(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	registry := module.Registry{}
	backend := &silentPlayback{}
	var player *Player
	err = module.RegisterMod(&registry, "speaker", func() module.Signal { return module.Signal{} }, module.Lifecycle[module.Signal]{Install: func(c *module.Context, _ *module.Signal) error {
		player = &Player{context: c, player: backend, suspended: !c.Active()}
		if err := module.Own(c, player); err != nil {
			return err
		}
		if err := player.Play(); err != nil {
			return err
		}
		if backend.playing {
			t.Fatal("startup played before scene activation")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := module.New(ecs.NewWorld(), plan, registry, module.Options{Source: files})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if _, err = r.Manager().StartEntry(); err != nil {
		t.Fatal(err)
	}
	if !player.IsPlaying() || backend.plays != 1 {
		t.Fatal("entry activation did not start intended playback")
	}
	if err = player.Seek(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	switchTo := func(scene project.SceneID) {
		t.Helper()
		if err := r.Manager().Switch(module.SceneSwitchRequest{Scene: scene, Retain: true}); err != nil {
			t.Fatal(err)
		}
		if err := r.Dispatch(); err != nil {
			t.Fatal(err)
		}
	}
	switchTo("game.json")
	if player.IsPlaying() || player.Position() != 5*time.Second {
		t.Fatal("suspend lost playback cursor")
	}
	switchTo("main.json")
	if !player.IsPlaying() || backend.plays != 2 || player.Position() != 5*time.Second {
		t.Fatal("resume recreated playback")
	}
	switchTo("game.json")
	if err = player.Pause(); err != nil {
		t.Fatal(err)
	}
	switchTo("main.json")
	if player.IsPlaying() || backend.plays != 2 {
		t.Fatal("explicit pause was undone by resume")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = player.Position(); _ = player.IsPlaying(); _ = player.Volume() }); allocs != 0 {
		t.Fatalf("playback polling allocated %g", allocs)
	}
	if err = player.Close(); err != nil {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	if backend.closes != 1 || player.Play() != module.ErrDisposed {
		t.Fatal("closed player remained usable or closed twice")
	}
}
