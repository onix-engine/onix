package editor

import (
	"io"
	"io/fs"
	"os"
)

const workspaceStagingDirectory = "resources/.editor-staging"

func (w *WorkspaceDocument) newAssetSnapshot() (string, error) {
	if w.stagingDir == "" {
		root := w.Absolute(workspaceStagingDirectory)
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
		var err error
		w.stagingDir, err = os.MkdirTemp(root, "workspace-")
		if err != nil {
			return "", err
		}
	}
	return os.MkdirTemp(w.stagingDir, "copy-")
}

// Dispose removes only this document's temporary copies. Saved editor trash
// remains recoverable independently of the editor session.
func (w *WorkspaceDocument) Dispose() error {
	if w == nil || w.stagingDir == "" {
		return nil
	}
	if err := os.RemoveAll(w.stagingDir); err != nil {
		return err
	}
	w.stagingDir = ""
	return nil
}

func copyWorkspaceAsset(source fs.FS, name, destination string) error {
	input, err := source.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	return writeAtomicOutput(destination, func(output io.Writer) error {
		_, err := io.Copy(output, input)
		return err
	})
}
