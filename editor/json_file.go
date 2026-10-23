package editor

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	projectFileName = "project.json"
)

func readJSONFile[T any](path string) (T, error) {
	var target T
	data, err := os.ReadFile(path)
	if err != nil {
		return target, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return target, fmt.Errorf("decode %s: file is empty", filepath.Base(path))
	}
	decoder := stdjson.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err != nil {
		return target, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return target, fmt.Errorf("decode %s: multiple JSON values are not allowed", filepath.Base(path))
		}
		return target, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return target, nil
}

func readOptionalJSONFile[T any](path string) (T, error) {
	value, err := readJSONFile[T](path)
	if os.IsNotExist(err) {
		return value, nil
	}
	return value, err
}

func writeJSONFile(path string, value any) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := stdjson.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
