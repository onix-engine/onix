package resources

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

func TestFileIconsAtlasBoundsAndSharedTexture(t *testing.T) {
	config, _, err := image.DecodeConfig(bytes.NewReader(fileIconsPNG))
	if err != nil {
		t.Fatal(err)
	}
	var metadata graphics.AtlasMetadata
	if err := json.Unmarshal(fileIconsMetadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Meta.Size.W != config.Width || metadata.Meta.Size.H != config.Height || len(metadata.Frames) != 16 {
		t.Fatal("atlas does not match the supplied icon sheet")
	}
	pixels, err := png.Decode(bytes.NewReader(fileIconsPNG))
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	base := FileIconTexture("json")
	for _, frame := range metadata.Frames {
		bounds := frame.Bounds
		if seen[frame.Filename] || bounds.X < 0 || bounds.Y < 0 || bounds.W <= 0 || bounds.H <= 0 || bounds.X+bounds.W > config.Width || bounds.Y+bounds.H > config.Height {
			t.Fatalf("invalid icon frame: %+v", frame)
		}
		seen[frame.Filename] = true
		_, _, _, alpha := pixels.At(bounds.X+bounds.W/2, bounds.Y+bounds.H/2).RGBA()
		if alpha == 0 {
			t.Fatalf("icon %s has no artwork at its center", frame.Filename)
		}
		texture := FileIconTexture(frame.Filename)
		if texture == nil || texture.BatchKey() != base.BatchKey() || texture.Width != bounds.W || texture.Height != bounds.H {
			t.Fatalf("icon %s does not reuse the atlas texture", frame.Filename)
		}
	}
	if FileIconTexture("missing") != nil {
		t.Fatal("unknown icon resolved to an unrelated frame")
	}
	if allocations := testing.AllocsPerRun(100, func() { FileIconTexture("image") }); allocations != 0 {
		t.Fatalf("cached icon lookup allocated %g", allocations)
	}
}

type fileIconUploadDriver struct {
	render.Driver
	uploads uint32
}

func (driver *fileIconUploadDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return render.TextureHandle{}, err
	}
	driver.uploads++
	return render.NewHandle[render.TextureResource](driver.uploads, 1), nil
}

func (*fileIconUploadDriver) Destroy() {}

func TestFileIconsUploadAfterDeviceInitializationAndRecreation(t *testing.T) {
	previous := render.ActiveDevice()
	t.Cleanup(previous.Activate)
	var noDevice *render.Device
	noDevice.Activate()
	icon := FileIconTexture("json")
	if icon.RenderHandle().Valid() {
		t.Fatal("icon has a GPU handle before device initialization")
	}
	for range 2 {
		driver := &fileIconUploadDriver{}
		device, err := render.NewDevice(driver)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(device.Destroy)
		device.Activate()
		handle := icon.RenderHandle()
		if !handle.Valid() || icon.UploadError() != nil {
			t.Fatalf("file icon was not uploaded: handle=%s error=%v", handle, icon.UploadError())
		}
		for _, name := range []string{"json", "image", "folder", "script"} {
			if texture := FileIconTexture(name); texture.RenderHandle() != handle {
				t.Fatalf("icon %s did not share the uploaded atlas", name)
			}
		}
		if driver.uploads != 1 || FileIconTexture("json") != icon {
			t.Fatal("device upload rebuilt the atlas or uploaded individual icons")
		}
		if allocations := testing.AllocsPerRun(100, func() { FileIconTexture("json").RenderHandle() }); allocations != 0 {
			t.Fatalf("uploaded icon lookup allocated %g", allocations)
		}
		device.Destroy()
	}
}

func TestFileIconStructuredPathsAndFallback(t *testing.T) {
	for _, test := range []struct {
		path string
		dir  bool
		want string
	}{
		{"assets.json", true, "folder"},
		{"button/mod.json", false, "prefab"},
		{"resources/scenes/main.json", false, "scene"},
		{"levels/main.scene.json", false, "scene"},
		{"button/private/data.json", false, "json"},
		{"assets/TEXTURE.PNG", false, "image"},
		{"assets/walk.anim.json", false, "animation"},
		{"assets/terrain.tileset.json", false, "tile"},
		{"assets/no-extension", false, "binary"},
	} {
		if got := FileIconName(test.path, test.dir); got != test.want {
			t.Errorf("%s: got %s, want %s", test.path, got, test.want)
		}
	}
}
