package graphics

import (
	"bytes"
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/diiyw/onix/render"
)

type retryUploadDriver struct {
	textureTestDriver
	fail bool
}

var errTestTextureUpload = errors.New("test upload failure")

func (d *retryUploadDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	handle, err := d.textureTestDriver.CreateTexture(descriptor, pixels)
	if d.fail {
		return render.TextureHandle{}, errTestTextureUpload
	}
	return handle, err
}

func TestTextureUploadScratchBudgetAndIndependentBorrowers(t *testing.T) {
	first, second := acquireTextureUploadScratch(128), acquireTextureUploadScratch(128)
	if first == second || &first.pixels[0] == &second.pixels[0] {
		t.Fatal("active uploads share temporary pixels")
	}
	releaseTextureUploadScratch(second)
	first.pixels = make([]byte, textureUploadScratchLimit+1)
	releaseTextureUploadScratch(first)
	if first.pixels != nil {
		t.Fatal("oversized upload buffer was retained")
	}
}

func TestCompactTextureUploadRetryClearsPooledSuffix(t *testing.T) {
	driver := &retryUploadDriver{fail: true}
	device, err := render.NewDevice(driver)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Destroy()
	texture := CreateEmptyTexture(32, 16, render.TextureFormatR8, ResourceUsageDefault)
	defer texture.Dispose()
	texture.pix = []byte{17, 231, 97}
	want := make([]byte, 32*16)
	copy(want, texture.pix)
	for attempt := range 3 {
		// Seed stale data even when the preceding upload failed or GC evicted
		// earlier entries. Neither path may expose it as untouched texels.
		scratch := acquireTextureUploadScratch(len(want))
		for i := range scratch.pixels {
			scratch.pixels[i] = 173
		}
		releaseTextureUploadScratch(scratch)
		handle := texture.RenderHandleFor(device)
		if !bytes.Equal(driver.createdPixels, want) || !bytes.Equal(texture.pix, want[:3]) || cap(texture.pix) != 3 {
			t.Fatal("upload changed the compact shadow or leaked stale pixels")
		}
		if attempt < 2 {
			if handle.Valid() || texture.UploadError() != errTestTextureUpload {
				t.Fatal("failed upload became resident or lost its error")
			}
			driver.fail = attempt == 0
			runtime.GC()
		} else if !handle.Valid() || texture.UploadError() != nil {
			t.Fatal("upload did not recover after driver failure")
		}
	}
	// Recycling staging bytes cannot mutate the driver's copied payload.
	scratch := acquireTextureUploadScratch(len(want))
	clear(scratch.pixels)
	releaseTextureUploadScratch(scratch)
	if !bytes.Equal(driver.createdPixels, want) {
		t.Fatal("device retained borrowed staging pixels")
	}
}

func TestConcurrentCompactTextureUploads(t *testing.T) {
	var wait sync.WaitGroup
	for index := range 8 {
		wait.Go(func() {
			driver := &textureTestDriver{}
			device, err := render.NewDevice(driver)
			if err != nil {
				t.Error(err)
				return
			}
			defer device.Destroy()
			for pass := range 8 {
				texture := CreateEmptyTexture(64, 32, render.TextureFormatRGBA8, ResourceUsageDefault)
				texture.pix = []byte{byte(index*17 + pass + 1)}
				if !texture.RenderHandleFor(device).Valid() || driver.createdPixels[0] != texture.pix[0] {
					t.Error("concurrent upload lost its prefix")
				}
				for _, value := range driver.createdPixels[1:] {
					if value != 0 {
						t.Error("concurrent upload retained another texture's pixels")
						break
					}
				}
				texture.Dispose()
			}
		})
	}
	wait.Wait()
}
