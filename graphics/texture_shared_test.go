package graphics

import (
	"bytes"
	"image"
	"testing"

	"github.com/diiyw/onix/render"
)

type sharedImageDriver struct {
	textureTestDriver
	updated []byte
}

func (d *sharedImageDriver) UpdateTextures(updates []render.TextureUpdate) error {
	for _, update := range updates {
		d.updated = append(d.updated[:0], update.Pixels...)
	}
	return nil
}

func TestSharedImageUpdatesEveryDeviceAndRestoresPixels(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 8, 4))
	texture := CreateTextureBySharedImage(im, false, ResourceUsageDefault)
	defer texture.Dispose()
	if &texture.pix[0] != &im.Pix[0] {
		t.Fatal("shared image copied its CPU shadow")
	}
	var drivers [3]sharedImageDriver
	for index := range 2 {
		device, err := render.NewDevice(&drivers[index])
		if err != nil {
			t.Fatal(err)
		}
		defer device.Destroy()
		if !texture.RenderHandleFor(device).Valid() {
			t.Fatal("initial shared image upload failed")
		}
	}
	for index := range im.Pix {
		im.Pix[index] = byte(index*37 + 1)
	}
	updates := []TextureRegionUpdate{{Width: 8, Height: 4, Pixels: im.Pix}}
	texture.UpdateRegions(updates)
	for index := range 2 {
		if !bytes.Equal(drivers[index].updated, im.Pix) {
			t.Fatal("shared image did not update every resident device")
		}
	}
	device, err := render.NewDevice(&drivers[2])
	if err != nil {
		t.Fatal(err)
	}
	defer device.Destroy()
	if !texture.RenderHandleFor(device).Valid() || !bytes.Equal(drivers[2].createdPixels, im.Pix) {
		t.Fatal("replacement device did not restore current shared pixels")
	}
	if allocs := testing.AllocsPerRun(100, func() { texture.UpdateRegions(updates) }); allocs != 0 {
		t.Fatalf("warm shared image update allocated %g times", allocs)
	}
}

func TestSharedImageNormalizesSubimagesAndPreservesSnapshotConstructor(t *testing.T) {
	base := image.NewRGBA(image.Rect(-3, 4, 13, 14))
	for index := range base.Pix {
		base.Pix[index] = byte(index)
	}
	im := base.SubImage(image.Rect(0, 6, 8, 10)).(*image.RGBA)
	texture := CreateTextureBySharedImage(im, false, ResourceUsageDefault)
	defer texture.Dispose()
	for y := range im.Rect.Dy() {
		offset := im.PixOffset(im.Rect.Min.X, im.Rect.Min.Y+y)
		if !bytes.Equal(texture.pix[y*32:(y+1)*32], im.Pix[offset:offset+32]) {
			t.Fatal("subimage upload included surrounding pixels or padding")
		}
	}
	im = image.NewRGBA(image.Rect(0, 0, 2, 2))
	snapshot := CreateTextureByImageNoMipmap(im)
	defer snapshot.Dispose()
	im.Pix[0] = 231
	if snapshot.pix[0] != 0 {
		t.Fatal("snapshot constructor began sharing caller memory")
	}
}
