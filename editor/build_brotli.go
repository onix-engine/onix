package editor

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/diiyw/onix/internal/brotli"
)

// compressWasmBrotli streams the raw WASM into its transport artifact so the
// build does not retain both the raw and compressed modules in memory.
func compressWasmBrotli(sourcePath string) (string, error) {
	targetPath := sourcePath + ".br"
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open raw WASM file: %w", err)
	}

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", errors.Join(fmt.Errorf("create WASM Brotli file: %w", err), source.Close())
	}
	writer := brotli.NewWriterLevel(target, brotli.BestCompression)
	_, copyErr := io.Copy(writer, source)
	writerErr := writer.Close()
	targetErr := target.Close()
	sourceErr := source.Close()
	if err := errors.Join(copyErr, writerErr, targetErr, sourceErr); err != nil {
		removeErr := os.Remove(targetPath)
		if os.IsNotExist(removeErr) {
			removeErr = nil
		}
		return "", errors.Join(fmt.Errorf("compress WASM file: %w", err), removeErr)
	}
	if err := os.Remove(sourcePath); err != nil {
		removeErr := os.Remove(targetPath)
		if os.IsNotExist(removeErr) {
			removeErr = nil
		}
		return "", errors.Join(fmt.Errorf("remove raw WASM file: %w", err), removeErr)
	}
	return targetPath, nil
}
