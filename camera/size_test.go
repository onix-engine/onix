//go:build js && !wechat

package camera

import "testing"

func TestFitSize(t *testing.T) {
	for _, test := range []struct {
		width, height int
		wantWidth     int
		wantHeight    int
	}{
		{width: 640, height: 480, wantWidth: 640, wantHeight: 480},
		{width: 1920, height: 1080, wantWidth: 960, wantHeight: 540},
		{width: 1080, height: 1920, wantWidth: 405, wantHeight: 720},
	} {
		width, height := fitSize(test.width, test.height, 960, 720)
		if width != test.wantWidth || height != test.wantHeight {
			t.Fatalf("fitSize(%d, %d) = %dx%d, want %dx%d", test.width, test.height, width, height, test.wantWidth, test.wantHeight)
		}
	}
}
