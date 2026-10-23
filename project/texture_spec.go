package project

import (
	"fmt"
	"strings"
)

type TextureSpec struct {
	Image  string     `json:"image,omitempty"`
	Atlas  string     `json:"atlas,omitempty"`
	Frame  string     `json:"frame,omitempty"`
	Color  string     `json:"color,omitempty"`
	Slice  *NineSlice `json:"slice,omitempty"`
	Mipmap bool       `json:"mipmap,omitempty"`
}

func (s *TextureSpec) validate(allowSlice bool) error {
	if s == nil {
		return nil
	}
	image := strings.TrimSpace(s.Image) != ""
	atlas := strings.TrimSpace(s.Atlas) != ""
	color := strings.TrimSpace(s.Color) != ""
	if countTrue(image, atlas, color) != 1 {
		return fmt.Errorf("requires exactly one of image, atlas, or color")
	}
	if image {
		normalized, err := NormalizeResourcePath(s.Image)
		if err != nil {
			return fmt.Errorf("image: %w", err)
		}
		s.Image = normalized
	}
	if atlas {
		normalized, err := NormalizeResourcePath(s.Atlas)
		if err != nil {
			return fmt.Errorf("atlas: %w", err)
		}
		s.Atlas = normalized
	}
	if atlas && strings.TrimSpace(s.Frame) == "" {
		return fmt.Errorf("atlas texture requires frame")
	}
	if !atlas && strings.TrimSpace(s.Frame) != "" {
		return fmt.Errorf("non-atlas texture cannot define frame")
	}
	if s.Slice != nil {
		if !allowSlice {
			return fmt.Errorf("particle texture cannot define slice")
		}
		if color {
			return fmt.Errorf("color texture cannot define slice")
		}
	}
	return nil
}

type NineSlice struct {
	Left   int `json:"left"`
	Right  int `json:"right"`
	Top    int `json:"top"`
	Bottom int `json:"bottom"`
}

func (s NineSlice) Validate(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("nine-slice texture dimensions must be positive")
	}
	if s.Left < 0 || s.Right < 0 || s.Top < 0 || s.Bottom < 0 {
		return fmt.Errorf("nine-slice insets cannot be negative")
	}
	if s.Left+s.Right >= width {
		return fmt.Errorf("nine-slice horizontal insets must leave a stretchable center")
	}
	if s.Top+s.Bottom >= height {
		return fmt.Errorf("nine-slice vertical insets must leave a stretchable center")
	}
	return nil
}
