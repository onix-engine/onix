package editor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const (
	editorSettingsFileName = "editor.json"
	recentProjectLimit     = 8
)

type editorSettings struct {
	RecentProjects      []string           `json:"recentProjects,omitempty"`
	FilePickerDirectory string             `json:"filePickerDirectory,omitempty"`
	OpenAI              openAISettings     `json:"openAI,omitzero"`
	Viewport3D          viewport3DSettings `json:"viewport3D,omitempty"`
	SnapToGrid          bool               `json:"snapToGrid,omitempty"`
}

type openAISettings struct {
	BaseURL string `json:"baseURL,omitempty"`
	APIKey  string `json:"apiKey,omitempty"`
	Model   string `json:"model,omitempty"`
}

func editorSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".onix", editorSettingsFileName), nil
}

func loadEditorSettings() (editorSettings, error) {
	path, err := editorSettingsPath()
	if err != nil {
		return editorSettings{}, err
	}
	var settings editorSettings
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return settings, nil
}

func saveEditorSettings(settings editorSettings) error {
	path, err := editorSettingsPath()
	if err != nil {
		return err
	}
	data, err := documentJSON(settings)
	if err != nil {
		return err
	}
	// Replace the settings using a private temporary file because they may contain credentials.
	return writeWorkspaceFile(path, data)
}

func defaultFilePickerDirectory(settings editorSettings, fallback string) string {
	if directory := existingDirectory(settings.FilePickerDirectory); directory != "" {
		return directory
	}
	if directory := existingDirectory(fallback); directory != "" {
		return directory
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func (settings *editorSettings) rememberFilePickerDirectory(directory string) error {
	directory = existingDirectory(directory)
	if directory == "" {
		return fmt.Errorf("file picker directory is not available")
	}
	if settings.FilePickerDirectory == directory {
		return nil
	}
	next := *settings
	next.FilePickerDirectory = directory
	if err := saveEditorSettings(next); err != nil {
		return err
	}
	*settings = next
	return nil
}

func (settings *editorSettings) rememberRecentProject(directory string) error {
	directory = existingDirectory(directory)
	if directory == "" {
		return fmt.Errorf("project directory is not available")
	}
	next := *settings
	next.RecentProjects = prependRecentProject(next.RecentProjects, directory)
	if slices.Equal(next.RecentProjects, settings.RecentProjects) {
		return nil
	}
	if err := saveEditorSettings(next); err != nil {
		return err
	}
	*settings = next
	return nil
}

func (settings *editorSettings) forgetRecentProject(directory string) error {
	next := *settings
	next.RecentProjects = slices.DeleteFunc(slices.Clone(settings.RecentProjects), func(item string) bool {
		return samePath(item, directory)
	})
	if slices.Equal(next.RecentProjects, settings.RecentProjects) {
		return nil
	}
	if err := saveEditorSettings(next); err != nil {
		return err
	}
	*settings = next
	return nil
}

func prependRecentProject(projects []string, directory string) []string {
	result := make([]string, 1, min(len(projects)+1, recentProjectLimit))
	result[0] = directory
	for _, item := range projects {
		if samePath(item, directory) {
			continue
		}
		result = append(result, item)
		if len(result) == recentProjectLimit {
			break
		}
	}
	return result
}

func samePath(left, right string) bool {
	left = filepath.Clean(strings.TrimSpace(left))
	right = filepath.Clean(strings.TrimSpace(right))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func existingDirectory(directory string) string {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return ""
	}
	abs, err := filepath.Abs(directory)
	if err != nil {
		return ""
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return ""
	}
	return filepath.Clean(abs)
}

func fileDialogDirectory(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return filepath.Dir(path)
	}
	return path
}
