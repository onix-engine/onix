package editor

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/resource"
)

func TestTemplateCatalogDecoders(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required to execute template catalog decoders")
	}
	data, err := resource.MarshalCatalog(resource.Catalog{
		Version: resource.CatalogVersion,
		Packages: []resource.PackageInfo{
			{
				Name: "base", File: "base.dat", SHA256: [32]byte{1}, Size: 256, Required: true,
				Assets: []resource.AssetInfo{
					{Path: "resources/project.json", Size: 300, Checksum: 0x12345678, ContentID: resource.ContentID{1}},
					{Path: "resources/海岸.svg", Size: 16384, ContentID: resource.ContentID{2}},
				},
			},
			{
				Name: "chapter", File: "chapter.dat", SHA256: [32]byte{2}, Size: 65536,
				Dependencies: []string{"base"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		end  string
	}{
		{"templates/web/index.html", "const loadResources ="},
		{"templates/wechat/game.js", "const catalogData ="},
	} {
		t.Run(test.path, func(t *testing.T) {
			source, err := buildTemplates.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			_, decoder, ok := strings.Cut(string(source), "const decodeCatalog =")
			if !ok {
				t.Fatal("catalog decoder is missing")
			}
			decoder, _, ok = strings.Cut(decoder, test.end)
			if !ok {
				t.Fatal("catalog decoder boundary is missing")
			}
			script := "const textEncoder = {TextDecoder};\nconst decodeCatalog =" + decoder + `
const assert = require("node:assert/strict");
const data = new Uint8Array(require("node:fs").readFileSync(0));
assert.deepEqual(decodeCatalog(data), [
  {name: "base", file: "base.dat", required: true, dependencies: []},
  {name: "chapter", file: "chapter.dat", required: false, dependencies: ["base"]},
]);
for (let length = 0; length < data.length; length++) {
  assert.throws(() => decodeCatalog(data.subarray(0, length)), /truncated resource catalog/);
}
const corrupt = data.slice();
corrupt[0] = 0;
assert.throws(() => decodeCatalog(corrupt), /invalid resource catalog marker/);
corrupt.set(new TextEncoder().encode("ONIXCAT1"));
assert.throws(() => decodeCatalog(corrupt), /invalid resource catalog marker/);
corrupt.set(data);
corrupt[8] = 1;
assert.throws(() => decodeCatalog(corrupt), /unsupported resource catalog version/);
assert.throws(() => decodeCatalog(new Uint8Array([...data, 0])), /trailing data/);
`
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, node, "-e", script)
			command.Stdin = bytes.NewReader(data)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("catalog decoder failed: %v\n%s", err, output)
			}
		})
	}
}
