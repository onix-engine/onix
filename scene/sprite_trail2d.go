package scene

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
)

// SpriteTrail2D records resolved transform snapshots and renders them as fading afterimages.
// Component presence enables the effect; remove it to disable and discard the trail.
type SpriteTrail2D struct {
	samples      []spriteTrailSample2D
	MaxSamples   int
	head         int
	lastPosition mgl32.Vec2
	Lifetime     float32
	Interval     float32
	MinDistance  float32
	AlphaStart   float32
	AlphaEnd     float32
	elapsed      float32
	hasLast      bool
}

type spriteTrailSample2D struct {
	sprite    Sprite2D
	transform mgl32.Mat4
	clip      clipRect2D
	anchor    Anchor
	width     float32
	height    float32
	age       float32
}

func NewSpriteTrail2D() SpriteTrail2D {
	return SpriteTrail2D{
		Lifetime:    0.45,
		Interval:    0.04,
		MinDistance: 3,
		AlphaStart:  0.65,
		MaxSamples:  24,
	}
}

func UpdateSpriteTrail2DSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "update_sprite_trail_2d"
	}
	return ecs.SystemConfig{Name: name, System: updateSpriteTrails2D}
}

func updateSpriteTrails2D(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	dt := float32(1.0 / 60.0)
	if clock, ok := world.GetResource[*Time](); ok && clock != nil && clock.Delta > 0 {
		dt = clock.DeltaSeconds()
	}
	for entity, access := range world.Query[SpriteTrail2D]().All {
		if !IsActive(world, entity) {
			continue
		}
		trail := access.Mut()

		sprite := world.GetRef[Sprite2D](entity)
		if sprite == nil {
			trail.Clear()
			continue
		}

		trail.normalize()
		trail.advance(dt)
		trail.elapsed += dt
		visibility := world.GetRef[Visibility](entity)
		if visibility == nil || !visibility.Visible {
			continue
		}
		if trail.sampleForTransform[GlobalTransform](world, entity, sprite) {
			continue
		}
		trail.sampleForTransform[UIGlobalTransform](world, entity, sprite)
	}
	return nil
}

func (trail *SpriteTrail2D) sampleForTransform[T renderGlobalTransform2D](world *ecs.World, entity ecs.Entity, sprite *Sprite2D) bool {
	transform := world.GetRef[T](entity)
	if transform == nil {
		return false
	}
	matrix := (*transform).renderMatrix2D()
	position := mgl32.Vec2{matrix[12], matrix[13]}
	if !trail.shouldSample(position) {
		return true
	}

	width, height := sprite.Size()
	width, height = renderSize2D(world, entity, width, height)
	if width <= 0 || height <= 0 || (sprite.Shape.Kind == 0 && (sprite.Texture == nil || !sprite.Texture.ValidFor(graphics.DeviceFor(world)))) {
		return true
	}
	trail.appendSample(spriteTrailSample2D{
		sprite:    cloneSprite2D(*sprite),
		transform: matrix,
		clip:      (*transform).renderClip2D(),
		anchor:    anchorForEntity2D(world, entity),
		width:     width,
		height:    height,
	})
	trail.elapsed = 0
	trail.lastPosition = position
	trail.hasLast = true
	return true
}

func (t *SpriteTrail2D) SampleCount() int {
	if t == nil {
		return 0
	}
	return len(t.samples)
}

func (t *SpriteTrail2D) Clear() {
	if t == nil {
		return
	}
	t.discard(len(t.samples))
	t.elapsed = 0
	t.lastPosition = mgl32.Vec2{}
	t.hasLast = false
}

// Reserve allocates storage for up to maxSamples trail snapshots. It is useful
// for pooled effects that need allocation-free updates after activation.
func (t *SpriteTrail2D) Reserve(maxSamples int) {
	if t == nil || maxSamples <= cap(t.samples) {
		return
	}
	t.grow(maxSamples)
}

func (t *SpriteTrail2D) normalize() {
	if t.Lifetime <= 0 {
		t.Lifetime = 0.45
	}
	if t.Interval < 0 {
		t.Interval = 0
	}
	if t.MinDistance < 0 {
		t.MinDistance = 0
	}
	t.AlphaStart = min(max(t.AlphaStart, float32(0)), float32(1))
	t.AlphaEnd = min(max(t.AlphaEnd, float32(0)), float32(1))
	if t.MaxSamples <= 0 {
		t.MaxSamples = 24
	}
}

func (t *SpriteTrail2D) advance(dt float32) {
	expired := 0
	for index := range len(t.samples) {
		sample := t.sample(index)
		sample.age += dt
		// Samples are appended in age order, so expiration removes a prefix.
		if index == expired && sample.age >= t.Lifetime {
			*sample = spriteTrailSample2D{}
			expired++
		}
	}
	t.advanceHead(expired)
}

func (t *SpriteTrail2D) shouldSample(position mgl32.Vec2) bool {
	if !t.hasLast {
		return true
	}
	if t.elapsed < t.Interval {
		return false
	}
	delta := position.Sub(t.lastPosition)
	return delta.Dot(delta) >= t.MinDistance*t.MinDistance
}

func (t *SpriteTrail2D) appendSample(sample spriteTrailSample2D) {
	if len(t.samples) >= t.MaxSamples {
		t.discard(len(t.samples) - t.MaxSamples + 1)
	}
	if len(t.samples) == cap(t.samples) {
		t.grow(min(t.MaxSamples, max(1, cap(t.samples)*2)))
	}
	index := len(t.samples)
	t.samples = t.samples[:index+1]
	*t.sample(index) = sample
}

func (t *SpriteTrail2D) sample(index int) *spriteTrailSample2D {
	index += t.head
	if index >= cap(t.samples) {
		index -= cap(t.samples)
	}
	return &t.samples[:cap(t.samples)][index]
}

func (t *SpriteTrail2D) discard(count int) {
	for index := range count {
		*t.sample(index) = spriteTrailSample2D{}
	}
	t.advanceHead(count)
}

func (t *SpriteTrail2D) advanceHead(count int) {
	if count == 0 {
		return
	}
	t.head = (t.head + count) % cap(t.samples)
	t.samples = t.samples[:len(t.samples)-count]
	if len(t.samples) == 0 {
		t.head = 0
	}
}

func (t *SpriteTrail2D) grow(capacity int) {
	values := make([]spriteTrailSample2D, len(t.samples), capacity)
	for index := range values {
		values[index] = *t.sample(index)
	}
	t.samples, t.head = values, 0
}

func (t *SpriteTrail2D) alphaAt(age float32) float32 {
	if t == nil || t.Lifetime <= 0 {
		return 0
	}
	progress := min(max(age/t.Lifetime, float32(0)), float32(1))
	return t.AlphaStart + (t.AlphaEnd-t.AlphaStart)*progress
}
