package project

import "fmt"

type TilemapSpec struct {
	ID         string    `json:"id"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	TileWidth  int       `json:"tileWidth"`
	TileHeight int       `json:"tileHeight"`
	Tileset    string    `json:"tileset,omitempty"`
	Tiles      []int     `json:"tiles"`
	TileRuns   []TileRun `json:"-"`
}

type TileRun struct {
	Value  int
	Length int
}

func (s TilemapSpec) validate() error {
	if s.Tileset == "" {
		return nil
	}
	if _, err := NormalizeResourcePath(s.Tileset); err != nil {
		return fmt.Errorf("tilemap %q tileset: %w", s.ID, err)
	}
	return nil
}

func (s TilemapSpec) Validate() error {
	return s.validate()
}

func (s *TilemapSpec) Normalize() {
	if s.Width <= 0 {
		s.Width = 16
	}
	if s.Height <= 0 {
		s.Height = 16
	}
	if s.TileWidth <= 0 {
		s.TileWidth = 32
	}
	if s.TileHeight <= 0 {
		s.TileHeight = 32
	}
	cellCount, ok := tilemapCellCount(s.Width, s.Height)
	if !ok {
		return
	}
	if len(s.Tiles) < cellCount {
		s.Tiles = append(s.Tiles, make([]int, cellCount-len(s.Tiles))...)
	} else if len(s.Tiles) > cellCount {
		s.Tiles = s.Tiles[:cellCount]
	}
}

func (s *TilemapSpec) Compact() {
	if s == nil || len(s.Tiles) == 0 {
		return
	}
	s.TileRuns = nil
	if len(s.Tiles) < 2 {
		return
	}

	runCount := 1
	previous := s.Tiles[0]
	for _, value := range s.Tiles[1:] {
		if value != previous {
			runCount++
			previous = value
		}
	}
	if runCount >= len(s.Tiles)-runCount {
		return
	}

	runs := make([]TileRun, 0, runCount)
	for _, value := range s.Tiles {
		if len(runs) == 0 || runs[len(runs)-1].Value != value {
			runs = append(runs, TileRun{Value: value, Length: 1})
			continue
		}
		runs[len(runs)-1].Length++
	}
	s.TileRuns = runs
	s.Tiles = nil
}

func (s *TilemapSpec) Expand() error {
	if s == nil {
		return nil
	}
	count, ok := tilemapCellCount(s.Width, s.Height)
	if !ok {
		return fmt.Errorf("tilemap %q has invalid dimensions", s.ID)
	}
	if len(s.TileRuns) == 0 {
		if len(s.Tiles) != count {
			return fmt.Errorf("tilemap %q has %d tiles, want %d", s.ID, len(s.Tiles), count)
		}
		return nil
	}
	if len(s.Tiles) != 0 {
		return fmt.Errorf("tilemap %q contains both tiles and tile runs", s.ID)
	}

	position := 0
	for _, run := range s.TileRuns {
		if run.Length <= 0 || run.Length > count-position {
			return fmt.Errorf("tilemap %q has invalid tile run length %d", s.ID, run.Length)
		}
		position += run.Length
	}
	if position != count {
		return fmt.Errorf("tilemap %q has %d missing tiles", s.ID, count-position)
	}

	tiles := make([]int, count)
	position = 0
	for _, run := range s.TileRuns {
		end := position + run.Length
		for index := position; index < end; index++ {
			tiles[index] = run.Value
		}
		position = end
	}
	s.Tiles = tiles
	s.TileRuns = nil
	return nil
}

func tilemapCellCount(width, height int) (int, bool) {
	if width <= 0 || height <= 0 {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	if width > maxInt/height {
		return 0, false
	}
	return width * height, true
}
