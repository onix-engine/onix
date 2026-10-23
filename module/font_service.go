package module

import (
	"crypto/sha256"
	"io"
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
)

type fontResource struct {
	runtime         *Runtime
	key             [32]byte
	font            *onixtext.Font
	providers, refs int
	outputs         map[TextRenderRequest]*fontOutput
	first, last     *fontOutput
}
type fontOutput struct {
	key            TextRenderRequest
	handle         RenderOutputHandle
	previous, next *fontOutput
}

func (f *fontResource) touch(o *fontOutput) {
	if f.first == o {
		return
	}
	if o.previous != nil {
		o.previous.next = o.next
	}
	if o.next != nil {
		o.next.previous = o.previous
	} else if f.last == o {
		f.last = o.previous
	}
	o.previous = nil
	o.next = f.first
	if f.first != nil {
		f.first.previous = o
	} else {
		f.last = o
	}
	f.first = o
}
func (f *fontResource) evict() error {
	o := f.last
	if o == nil {
		return nil
	}
	delete(f.outputs, o.key)
	f.last = o.previous
	if f.last != nil {
		f.last.next = nil
	} else {
		f.first = nil
	}
	return o.handle.Release()
}

// FontService reads only its module's declared resource. Identical font bytes
// share layout/output caches within the Runtime; provider state stays private.
type FontService struct{ resource *fontResource }

func (f *FontService) Load(c *Context, resource string) error {
	if f.resource != nil || !c.installing {
		return ErrInvalid
	}
	file, err := c.OpenResource(resource)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	err = joinError(err, file.Close())
	if err != nil {
		return err
	}
	key := sha256.Sum256(data)
	shared := c.runtime.fonts[key]
	if shared == nil {
		font, loadErr := onixtext.LoadFont(data)
		if loadErr != nil {
			return loadErr
		}
		shared = &fontResource{runtime: c.runtime, key: key, font: font, outputs: make(map[TextRenderRequest]*fontOutput)}
		c.runtime.fonts[key] = shared
	}
	shared.providers++
	shared.refs++
	f.resource = shared
	return nil
}

func (f *FontService) Render(c *Context, request Request[TextRenderRequest, TextRenderResult]) error {
	shared := f.resource
	if shared == nil || c == nil || c.runtime != shared.runtime {
		return ErrDisposed
	}
	value := request.Value
	if !validTextRequest(value) {
		return request.Fail(c, ErrInvalid)
	}
	cached := shared.outputs[value]
	if cached == nil {
		if len(shared.outputs) >= c.runtime.options.TextCacheCapacity {
			if err := shared.evict(); err != nil {
				return request.Fail(c, err)
			}
		}
		text := scene.Text2D{Value: value.Text, Font: shared.font, FontSize: value.Size, DPI: 72,
			Color: graphics.ColorRGBA{R: value.Color[0], G: value.Color[1], B: value.Color[2], A: value.Color[3]}, Alpha: 1,
			WrapWidth: value.Width, TextAlignment: value.Alignment}
		handle, err := c.runtime.outputs.TextBox(text, scene.Size2D{Width: value.Width, Height: value.Height}, shared.releaseOutput)
		if err != nil {
			return request.Fail(c, err)
		}
		shared.refs++
		cached = &fontOutput{key: value, handle: handle}
		shared.outputs[value] = cached
	}
	shared.touch(cached)
	lease, err := cached.handle.Retain()
	if err != nil {
		return request.Fail(c, err)
	}
	width, height, _ := lease.Size()
	if err = request.Reply(c, TextRenderResult{Output: lease, Width: width, Height: height}); err != nil {
		_ = lease.Release()
		return err
	}
	return nil
}

func validTextRequest(v TextRenderRequest) bool {
	if v.Size <= 0 || math.IsNaN(v.Size) || math.IsInf(v.Size, 0) || v.Width < 0 || v.Height < 0 || v.Alignment > scene.TextAlignRight {
		return false
	}
	for _, n := range [6]float32{v.Color[0], v.Color[1], v.Color[2], v.Color[3], v.Width, v.Height} {
		if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
			return false
		}
	}
	return true
}

func (f *FontService) Destroy() error {
	if f.resource == nil {
		return nil
	}
	shared := f.resource
	f.resource = nil
	shared.providers--
	var err error
	if shared.providers == 0 {
		for _, o := range shared.outputs {
			err = joinError(err, o.handle.Release())
		}
		clear(shared.outputs)
		shared.first, shared.last = nil, nil
	}
	return joinError(err, shared.releaseOutput())
}
func (f *fontResource) releaseOutput() error {
	f.refs--
	if f.refs != 0 {
		return nil
	}
	delete(f.runtime.fonts, f.key)
	return f.font.Dispose()
}
