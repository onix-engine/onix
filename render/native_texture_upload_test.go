//go:build cgo && (android || (linux && onix_drm && !onix_drm_vulkan) || darwin || ios)

package render

import "testing"

func TestNativeTextureUploadReusesAndReleasesStorage(t *testing.T) {
	var upload nativeTextureUpload
	buffer, ok := upload.reserve(32)
	if !ok || len(buffer) != 32 || upload.capacity < 32 {
		t.Fatal("native texture upload storage was not allocated")
	}
	buffer[0], buffer[31] = 17, 29
	if next, ok := upload.reserve(16); !ok || next[0] != 17 || upload.capacity < 32 {
		t.Fatal("native texture upload storage was not reused")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if _, ok := upload.reserve(32); !ok {
			panic("native texture upload storage was lost")
		}
	}); allocs != 0 {
		t.Fatalf("warm native upload allocated %g Go objects", allocs)
	}
	if _, ok := upload.reserve(128 << 10); !ok {
		t.Fatal("native texture upload storage did not grow")
	}
	for range bufferTrimDelay + 1 {
		upload.trim()
	}
	if upload.pointer != nil || upload.capacity != 0 {
		t.Fatal("idle native texture upload storage was retained")
	}
	upload.release()
}
