//go:build ignore

package main

import (
	"bytes"
	"log"
	"os"

	"github.com/onix-engine/onix/images/svg"
)

func main() {
	const name = "resources/icons/chevron-down"
	data, err := os.ReadFile(name + ".svg")
	if err != nil {
		log.Fatal(err)
	}
	icon, err := svg.Render(bytes.NewReader(data), svg.Options{})
	if err != nil {
		log.Fatal(err)
	}
	if icon.Rect.Dx() != 16 || icon.Rect.Dy() != 16 {
		log.Fatal("expected a 16x16 select indicator")
	}
	// Preserve the white edge color through transparent texels for filtering.
	for i := 0; i < len(icon.Pix); i += 4 {
		if icon.Pix[i+3] == 0 {
			icon.Pix[i], icon.Pix[i+1], icon.Pix[i+2] = 255, 255, 255
		}
	}
	if err := os.WriteFile(name+".rgba", icon.Pix, 0644); err != nil {
		log.Fatal(err)
	}
}
