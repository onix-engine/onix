package html

import (
	"fmt"
	"image"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/scene"
)

func TestHTMLObjectFitUpdatesRetainedImage(t *testing.T) {
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 100, 50)))
	t.Cleanup(func() { _ = texture.Dispose() })
	world, owner := incrementalHTMLFixture(t, `<img id="image" src="test.png" style="display:block;width:200px;height:200px">`)
	t.Cleanup(func() { Dispose(world) })
	world.InsertResource[ImageProvider](testHTMLImageProvider{texture: texture})
	incrementalSync(t, world)
	node := incrementalHTMLNode(t, world, owner, "image")
	var retained ecs.Entity
	var previousLayer *scene.SpriteLayer2D
	for _, test := range []struct {
		fit, position                            string
		boxWidth, boxHeight, x, y, width, height float32
	}{
		{"fill", "center", 200, 200, 0, 0, 200, 200},
		{"contain", "center", 200, 200, 0, 50, 200, 100},
		{"cover", "center", 200, 200, -100, 0, 400, 200},
		{"none", "right 10px bottom 20px", 200, 200, 90, 130, 100, 50},
		{"scale-down", "center", 40, 40, 0, 10, 40, 20},
		{"none", "center", 40, 40, -30, -5, 100, 50},
		{"fill", "10px -5px", 40, 40, 10, -5, 40, 40},
		{"fill", "center", 40, 40, 0, 0, 40, 40},
		{"contain", "center", 200, 100, 0, 0, 200, 100},
		{"scale-down", "center", 200, 100, 50, 25, 100, 50},
	} {
		t.Run(fmt.Sprintf("%s/%s/%gx%g", test.fit, test.position, test.boxWidth, test.boxHeight), func(t *testing.T) {
			node.SetAttribute("style", fmt.Sprintf("display:block;width:%gpx;height:%gpx;object-fit:%s;object-position:%s", test.boxWidth, test.boxHeight, test.fit, test.position))
			incrementalSync(t, world)
			state := world.GetRef[htmlRenderState](owner)
			var entity ecs.Entity
			for _, candidate := range state.RenderNodes[node.node].Entities {
				child, _ := world.Get[htmlChild](candidate)
				if child.role == htmlRoleImage {
					entity = candidate
					break
				}
			}
			if entity == 0 || retained != 0 && entity != retained {
				t.Fatal("object-fit replaced or removed the retained image")
			}
			retained = entity
			sprite, plain := world.Get[scene.Sprite2D](entity)
			x, y := float32(0), float32(0)
			layers, layered := world.Get[scene.SpriteLayers2D](entity)
			if layered {
				if plain || len(layers.Layers) != 1 {
					t.Fatal("fitted image has duplicate or missing sprites")
				}
				layer := &layers.Layers[0]
				if previousLayer != nil && layer != previousLayer {
					t.Fatal("fitted image update allocated a new layer buffer")
				}
				previousLayer = layer
				sprite, x, y = layer.Sprite, layer.Transform.X, layer.Transform.Y
				matrix := layers.LayerMatrix(0)
				if matrix[12] != test.x || matrix[13] != test.y {
					t.Fatalf("image matrix offset = %v,%v, want %v,%v", matrix[12], matrix[13], test.x, test.y)
				}
				if clip, _ := world.Get[scene.ClipRect2D](entity); clip != (scene.ClipRect2D{Width: test.boxWidth, Height: test.boxHeight}) {
					t.Fatalf("image clip = %+v", clip)
				}
			} else {
				previousLayer = nil
				if !plain || world.Has[scene.ClipRect2D](entity) {
					t.Fatal("unshifted fill retained a fitted layer or clip")
				}
			}
			if x != test.x || y != test.y || sprite.Width != test.width || sprite.Height != test.height {
				t.Fatalf("image rect = %v,%v %vx%v, want %v,%v %vx%v", x, y, sprite.Width, sprite.Height, test.x, test.y, test.width, test.height)
			}
			size, _ := world.Get[scene.Size2D](entity)
			target, _ := world.Get[scene.InputTarget2D](entity)
			if size != (scene.Size2D{Width: test.boxWidth, Height: test.boxHeight}) || target.Width != test.boxWidth || target.Height != test.boxHeight {
				t.Fatal("object-fit changed layout or hit bounds")
			}
		})
	}
	state := world.GetRef[htmlRenderState](owner)
	fragment := layout.ImageFragment{Node: node.node, Style: state.Styles[node.node], Rect: layout.Rect{Width: 200, Height: 100}}
	if allocations := testing.AllocsPerRun(100, func() {
		if entity, ok := spawnHTMLImage(world, owner, fragment, 1, true); !ok || entity != retained {
			panic("retained image missing")
		}
	}); allocations != 0 {
		t.Fatalf("warm object-fit rendering allocated %g times", allocations)
	}
}
