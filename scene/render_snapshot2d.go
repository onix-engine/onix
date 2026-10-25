package scene

import (
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

type renderSnapshots2DScratch struct{}

type renderSnapshots2D struct {
	sprites    []Sprite2D
	emitters   []ParticleEmitter2D
	tilemaps   []tilemapSnapshot2D
	texts      []Text2D
	spines     []Spine2D
	nineSlices []NineSlice2D

	particleColors      []graphics.ColorRGBA
	particleAppearances []ParticleAppearance2D
	particles           []particleState2D
	textSpans           []TextSpan2D
	inlineObjects       []TextInlineObject2D
	lastObjects         []TextInlineObject2D

	previousSprites     int
	previousEmitters    int
	previousTilemaps    int
	previousTexts       int
	previousSpines      int
	previousAppearances int
	previousParticles   int
}

func (s *renderSnapshots2D) reset() {
	s.previousSprites, s.previousEmitters = len(s.sprites), len(s.emitters)
	s.previousTilemaps, s.previousTexts = len(s.tilemaps), len(s.texts)
	s.previousSpines = len(s.spines)
	s.previousAppearances, s.previousParticles = len(s.particleAppearances), len(s.particles)
	s.sprites = s.sprites[:0]
	s.emitters = s.emitters[:0]
	s.tilemaps = s.tilemaps[:0]
	s.texts = s.texts[:0]
	s.spines = s.spines[:0]
	s.nineSlices = s.nineSlices[:0]
	s.particleColors = resetLayoutBuffer(s.particleColors, len(s.particleColors))
	s.particleAppearances = resetLayoutBuffer(s.particleAppearances, len(s.particleAppearances))
	s.particles = resetLayoutBuffer(s.particles, len(s.particles))
	s.textSpans = resetLayoutBuffer(s.textSpans, len(s.textSpans))
	s.inlineObjects = resetLayoutBuffer(s.inlineObjects, len(s.inlineObjects))
	s.lastObjects = resetLayoutBuffer(s.lastObjects, len(s.lastObjects))
}

func (s *renderSnapshots2D) releaseUnused() {
	clearSnapshotTail(s.sprites, s.previousSprites)
	clearSnapshotTail(s.emitters, s.previousEmitters)
	clearSnapshotTail(s.tilemaps, s.previousTilemaps)
	clearSnapshotTail(s.texts, s.previousTexts)
	clearSnapshotTail(s.spines, s.previousSpines)
	clearSnapshotTail(s.particleAppearances, s.previousAppearances)
	clearSnapshotTail(s.particles, s.previousParticles)
}

func clearSnapshotTail[T any](values []T, previousLen int) {
	end := min(previousLen, cap(values))
	if end > len(values) {
		clear(values[:end][len(values):])
	}
}

func (s *renderSnapshots2D) addSprite(value Sprite2D) *Sprite2D {
	if value.Slice != nil {
		s.nineSlices = append(s.nineSlices, *value.Slice)
		value.Slice = &s.nineSlices[len(s.nineSlices)-1]
	}
	s.sprites = append(s.sprites, value)
	return &s.sprites[len(s.sprites)-1]
}

func (s *renderSnapshots2D) addEmitter(value ParticleEmitter2D) *ParticleEmitter2D {
	value.Colors = appendSnapshotValues(&s.particleColors, value.Colors)
	value.Appearances = appendSnapshotValues(&s.particleAppearances, value.Appearances)
	value.particles = appendSnapshotValues(&s.particles, value.particles)
	s.emitters = append(s.emitters, value)
	return &s.emitters[len(s.emitters)-1]
}

func (s *renderSnapshots2D) addTilemap(value Tilemap2D) *Tilemap2D {
	index := len(s.tilemaps)
	var previous tilemapSnapshot2D
	if index < cap(s.tilemaps) {
		previous = s.tilemaps[:index+1][index]
	}
	var source *int
	if len(value.Tiles) != 0 {
		source = &value.Tiles[0]
	}
	if value.Revision != 0 && value.Revision == previous.value.Revision && previous.source == source && len(value.Tiles) == len(previous.value.Tiles) {
		value.Tiles = previous.value.Tiles
	} else {
		value.Tiles = append(previous.value.Tiles[:0], value.Tiles...)
	}
	s.tilemaps = append(s.tilemaps, tilemapSnapshot2D{value: value, source: source})
	return &s.tilemaps[len(s.tilemaps)-1].value
}

type tilemapSnapshot2D struct {
	value  Tilemap2D
	source *int
}

func (s *renderSnapshots2D) addText(value Text2D) *Text2D {
	value.Spans = appendSnapshotValues(&s.textSpans, value.Spans)
	value.InlineObjects = appendSnapshotValues(&s.inlineObjects, value.InlineObjects)
	value.lastInlineObjects = appendSnapshotValues(&s.lastObjects, value.lastInlineObjects)
	// glyphs, lines and inlineObjectLayouts are rebuilt by the text layout before
	// the render stage and only read while drawing, so the snapshot borrows those
	// arrays instead of duplicating a document-sized layout every frame.
	s.texts = append(s.texts, value)
	return &s.texts[len(s.texts)-1]
}

func (s *renderSnapshots2D) addSpine(value Spine2D) *Spine2D {
	index := len(s.spines)
	var batch render.MeshBatch3D
	if index < cap(s.spines) {
		batch = s.spines[:index+1][index].batch
	}
	s.spines = append(s.spines, value)
	s.spines[index].batch = batch
	return &s.spines[len(s.spines)-1]
}

func appendSnapshotValues[T any](buffer *[]T, values []T) []T {
	start := len(*buffer)
	*buffer = append(*buffer, values...)
	return (*buffer)[start:]
}
