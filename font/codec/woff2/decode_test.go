package woff2

import "testing"

func TestDecoderRejectsInvalidWOFF2(t *testing.T) {
	if _, err := (Decoder{}).DecodeFont([]byte("invalid"), "font.woff2"); err == nil {
		t.Fatal("invalid WOFF2 decoded successfully")
	}
}
