package editor

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

func ignoredResourceFile(entry fs.DirEntry) bool {
	return strings.HasSuffix(entry.Name(), ".go") || strings.EqualFold(filepath.Ext(entry.Name()), ".heapsnapshot")
}

func scannableResource(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json", ".gltf", ".glb", ".atlas", ".mtl", ".obj", ".glsl", ".vert", ".frag", ".metal", ".wgsl":
		return true
	default:
		return false
	}
}

func visitGLTFReferences(data []byte, visitor func(string) error) error {
	// Only buffer and image URIs are external glTF dependencies. Names and
	// extras can contain arbitrary text, including characters invalid in paths.
	type uriReference struct {
		URI string `json:"uri"`
	}
	var document struct {
		Buffers []uriReference `json:"buffers"`
		Images  []uriReference `json:"images"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode glTF references: %w", err)
	}
	for _, references := range [...][]uriReference{document.Buffers, document.Images} {
		for _, reference := range references {
			if reference.URI != "" {
				if err := visitor(reference.URI); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func visitJSONStrings(data []byte, visitor func(string) error) error {
	if err := walkJSONStrings(data, nil); err != nil {
		return nil
	}
	return walkJSONStrings(data, visitor)
}

func walkJSONStrings(data []byte, visitor func(string) error) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if value, ok := token.(string); ok && visitor != nil {
			if err := visitor(value); err != nil {
				return err
			}
		}
	}
}

func visitTextReferences(data []byte, visitor func(string) error) error {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if index := strings.Index(trimmed, "#include"); index >= 0 {
			rest := strings.TrimSpace(trimmed[index+len("#include"):])
			if len(rest) >= 2 && (rest[0] == '"' || rest[0] == '<') {
				end := rest[1:]
				if closing := strings.IndexAny(end, "\"><"); closing > 0 {
					if err := visitor(textReferencePath(end[:closing])); err != nil {
						return err
					}
				}
			}
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && (fields[0] == "mtllib" || strings.HasPrefix(fields[0], "map_")) {
			if err := visitor(textReferencePath(fields[len(fields)-1])); err != nil {
				return err
			}
		}
	}
	return nil
}

func glbJSON(data []byte) ([]byte, bool) {
	if len(data) < 12 || string(data[:4]) != "glTF" || binary.LittleEndian.Uint32(data[4:8]) != 2 {
		return nil, false
	}
	declaredSize := uint64(binary.LittleEndian.Uint32(data[8:12]))
	if declaredSize < 12 || declaredSize > uint64(len(data)) {
		return nil, false
	}
	for offset := uint64(12); offset+8 <= declaredSize; {
		chunkSize := uint64(binary.LittleEndian.Uint32(data[offset : offset+4]))
		chunkType := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		if chunkSize > declaredSize-offset-8 {
			return nil, false
		}
		chunk := data[offset+8 : offset+8+chunkSize]
		if chunkType == 0x4e4f534a { // JSON
			return bytes.TrimRight(chunk, " \t\r\n\x00"), true
		}
		offset += 8 + chunkSize
	}
	return nil, false
}

func looksLikeResourceReference(value string) bool {
	if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(strings.ToLower(value), "data:") || strings.Contains(value, "://") {
		return false
	}
	return strings.ContainsAny(value, "/\\") || filepath.Ext(value) != ""
}

func textReferencePath(value string) string {
	if filepath.Ext(value) == "" && !strings.ContainsAny(value, "/\\") {
		return "./" + value
	}
	return value
}

func pathWithinRoot(relative string) bool {
	return relative != ".." && !strings.HasPrefix(filepath.ToSlash(relative), "../")
}
