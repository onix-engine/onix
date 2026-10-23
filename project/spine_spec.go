package project

import (
	"fmt"
	"strings"
)

// SpineSpec groups the three files that form one Spine export.
type SpineSpec struct {
	ID       string `json:"id"`
	Skeleton string `json:"skeleton"`
	Atlas    string `json:"atlas"`
	Texture  string `json:"texture"`
}

func (s SpineSpec) validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("spine id is required")
	}
	if strings.TrimSpace(s.Skeleton) == "" || strings.TrimSpace(s.Atlas) == "" || strings.TrimSpace(s.Texture) == "" {
		return fmt.Errorf("spine %q requires skeleton, atlas, and texture files", s.ID)
	}
	for _, file := range [...]struct {
		name string
		path string
	}{
		{name: "skeleton", path: s.Skeleton},
		{name: "atlas", path: s.Atlas},
		{name: "texture", path: s.Texture},
	} {
		if _, err := NormalizeResourcePath(file.path); err != nil {
			return fmt.Errorf("spine %q %s: %w", s.ID, file.name, err)
		}
	}
	return nil
}

func (s SpineSpec) Validate() error { return s.validate() }
