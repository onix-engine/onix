package graphics

import (
	"errors"
	"runtime"
	"testing"
	"weak"

	"github.com/onix-engine/onix/internal/reuse"
	"github.com/onix-engine/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func abandonedColorTexture(t *testing.T, color ColorRGBA, devices ...*render.Device) *colorTextureEntry {
	t.Helper()
	texture := CreateTextureByColor(color)
	for _, device := range devices {
		if !texture.RenderHandleFor(device).Valid() {
			t.Fatal("color texture upload failed")
		}
	}
	return texture.colorOwner
}

func collectColorTexture(t *testing.T, entry *colorTextureEntry) {
	t.Helper()
	for range 10 {
		runtime.GC()
		if entry.texture.Value() == nil {
			return
		}
	}
	t.Fatal("color cache retained an unused CPU texture")
}

func sweepAllColorTextures(device *render.Device) error {
	colorTextures.Lock()
	defer colorTextures.Unlock()
	return sweepColorTextures(device, len(colorTextures.slots))
}

func TestColorTextureRetiresOnEachOwningDevice(t *testing.T) {
	a, b := &textureTestDriver{}, &textureTestDriver{}
	first, _ := render.NewDevice(a)
	second, _ := render.NewDevice(b)
	defer first.Destroy()
	defer second.Destroy()
	color := ColorFromRGBA(19, 43, 71, 213)
	entry := abandonedColorTexture(t, color, first, second)
	collectColorTexture(t, entry)
	if a.destroyedTextures != 0 || b.destroyedTextures != 0 {
		t.Fatal("GC called a GPU driver")
	}
	if err := sweepAllColorTextures(nil); err != nil || a.destroyedTextures != 0 || b.destroyedTextures != 0 {
		t.Fatal("CPU sweep called a GPU driver", err)
	}
	// Replacing a dead texture must not let its deferred cleanup erase the new
	// color lookup. The old record still has two devices to retire.
	replacement := CreateTextureByColor(color)
	if replacement.colorOwner == entry {
		t.Fatal("replacement reused an abandoned residency record")
	}
	if err := sweepAllColorTextures(first); err != nil || a.destroyedTextures != 1 || b.destroyedTextures != 0 {
		t.Fatal("first sweep crossed device ownership", err)
	}
	if err := sweepAllColorTextures(second); err != nil || a.destroyedTextures != 1 || b.destroyedTextures != 1 {
		t.Fatal("second residency was lost or retired twice", err)
	}
	if CreateTextureByColor(color) != replacement {
		t.Fatal("retiring old resources removed the replacement cache entry")
	}
	if n := testing.AllocsPerRun(100, func() {
		CreateTextureByColor(color)
		_ = maintainColorTextures(first)
	}); n != 0 {
		t.Fatalf("warm color lookup/maintenance allocated %g", n)
	}
	runtime.KeepAlive(replacement)
}

type colorRetirementDriver struct {
	textureTestDriver
	fail bool
}

var errColorRetirement = errors.New("test color retirement failure")

func (driver *colorRetirementDriver) DestroyTexture(handle render.TextureHandle) error {
	if driver.fail {
		return errColorRetirement
	}
	return driver.textureTestDriver.DestroyTexture(handle)
}

func TestColorTextureRetirementRetriesAndDropsDestroyedDevices(t *testing.T) {
	driver := &colorRetirementDriver{fail: true}
	device, _ := render.NewDevice(driver)
	defer device.Destroy()
	entry := abandonedColorTexture(t, ColorFromRGBA(37, 61, 89, 233), device)
	collectColorTexture(t, entry)
	if err := sweepAllColorTextures(device); !errors.Is(err, errColorRetirement) {
		t.Fatal("retirement failure was lost", err)
	}
	driver.fail = false
	if err := sweepAllColorTextures(device); err != nil || driver.destroyedTextures != 1 {
		t.Fatal("failed retirement lost its handle", err)
	}
	entry = abandonedColorTexture(t, ColorFromRGBA(41, 67, 97, 239), device)
	device.Destroy()
	collectColorTexture(t, entry)
	if err := sweepAllColorTextures(nil); err != nil || driver.destroyedTextures != 1 {
		t.Fatal("cleanup called a destroyed driver", err)
	}
	colorTextures.Lock()
	remaining := colorTextures.entries[entry.key] == entry
	colorTextures.Unlock()
	if remaining {
		t.Fatal("destroyed device pinned an abandoned cache record")
	}
}

func queueColorTexture(canvas *Canvas2D, color ColorRGBA, subtexture bool) weak.Pointer[Texture] {
	texture := CreateTextureByColor(color)
	if subtexture {
		canvas.BindTexture(NewSubTexture(texture, 1, 1, 0, 0, 1, 1, 0))
	} else {
		canvas.BindTexture(texture)
	}
	canvas.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	canvas.BindTexture(CreateTextureByColor(White))
	return weak.Make(texture)
}

func TestColorTextureSurvivesQueuedDrawUntilFlush(t *testing.T) {
	for _, name := range []string{"direct", "view"} {
		t.Run(name, func(t *testing.T) {
			testColorTextureQueuedDraw(t, name == "view")
		})
	}
}

func testColorTextureQueuedDraw(t *testing.T, subtexture bool) {
	driver := &textureTestDriver{}
	device, _ := render.NewDevice(driver)
	defer device.Destroy()
	canvas := NewCanvas2D(device, 100, 100)
	pointer := queueColorTexture(canvas, ColorFromRGBA(47, 73, 101, 241), subtexture)
	runtime.GC()
	if pointer.Value() == nil {
		t.Fatal("queued sprite lost its color texture owner")
	}
	if err := sweepAllColorTextures(device); err != nil || driver.destroyedTextures != 0 {
		t.Fatal("queued texture was retired before submission", err)
	}
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		runtime.GC()
		if pointer.Value() == nil {
			break
		}
	}
	if pointer.Value() != nil {
		t.Fatal("completed frame retained a color texture")
	}
	if err := canvas.FinishFrame(); err != nil || driver.destroyedTextures != 1 {
		t.Fatal("frame cleanup did not retire the abandoned texture", err)
	}
	runtime.KeepAlive(canvas)
}

func populateColorTexturePeak() {
	textures := make([]*Texture, 2048)
	for i := range textures {
		textures[i] = CreateTextureByColor(ColorFromRGBA(uint8(i), uint8(i>>8), 157, 254))
	}
	runtime.KeepAlive(textures)
}

func TestColorTextureRegistryReclaimsHistoricalPeak(t *testing.T) {
	populateColorTexturePeak()
	colorTextures.Lock()
	peak := cap(colorTextures.slots)
	count := len(colorTextures.slots)
	oldIndex := colorTextures.entries
	colorTextures.Unlock()
	runtime.GC()
	for range count/64 + reuse.RetentionFrames*2 + 2 {
		if err := maintainColorTextures(nil); err != nil {
			t.Fatal(err)
		}
	}
	colorTextures.Lock()
	defer colorTextures.Unlock()
	if cap(colorTextures.slots) >= peak/4 {
		t.Fatalf("color registry retained peak slots: %d -> %d", peak, cap(colorTextures.slots))
	}
	probe := &colorTextureEntry{}
	oldIndex[-1] = probe
	if colorTextures.entries[-1] == probe {
		delete(colorTextures.entries, -1)
		t.Fatal("color registry retained its historical map buckets")
	}
}
