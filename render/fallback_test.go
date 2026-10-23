package render

import (
	"errors"
	"strings"
	"testing"
)

func TestNewDriverWithFallbackUsesPrimary(t *testing.T) {
	want := &driverTestStub{}
	fallbackCalled := false
	got, err := newDriverWithFallback(7, "primary", func(value int) (Driver, error) {
		if value != 7 {
			t.Fatalf("surface = %d, want 7", value)
		}
		return want, nil
	}, "fallback", func(int) (Driver, error) {
		fallbackCalled = true
		return nil, errors.New("unexpected")
	})
	if err != nil || got != want || fallbackCalled {
		t.Fatalf("driver = %v, err = %v, fallback called = %t", got, err, fallbackCalled)
	}
}

func TestNewDriverWithFallbackUsesFallbackAfterFailure(t *testing.T) {
	want := &driverTestStub{}
	got, err := newDriverWithFallback(0, "Vulkan", func(int) (Driver, error) {
		return nil, errors.New("unavailable")
	}, "OpenGL ES", func(int) (Driver, error) {
		return want, nil
	})
	if err != nil || got != want {
		t.Fatalf("driver = %v, err = %v", got, err)
	}
}

func TestNewDriverWithFallbackReportsBothFailures(t *testing.T) {
	_, err := newDriverWithFallback(0, "Vulkan", func(int) (Driver, error) {
		return nil, errors.New("no loader")
	}, "OpenGL ES", func(int) (Driver, error) {
		return nil, errors.New("no EGL config")
	})
	if err == nil || !strings.Contains(err.Error(), "Vulkan driver: no loader") || !strings.Contains(err.Error(), "OpenGL ES fallback driver: no EGL config") {
		t.Fatalf("combined error = %v", err)
	}
}

type driverTestStub struct{}

func (*driverTestStub) RendererInfo() RendererInfo { return RendererInfo{} }
func (*driverTestStub) BeginFrame(Color) error     { return nil }
func (*driverTestStub) SetViewport(Rect)           {}
func (*driverTestStub) Viewport() Rect             { return Rect{} }
func (*driverTestStub) SetBlend(BlendState) error  { return nil }
func (*driverTestStub) SetScissor(*Rect)           {}
func (*driverTestStub) CreateTexture(TextureDescriptor, []byte) (TextureHandle, error) {
	return TextureHandle{}, nil
}
func (*driverTestStub) UpdateTextures([]TextureUpdate) error { return nil }
func (*driverTestStub) DestroyTexture(TextureHandle) error   { return nil }
func (*driverTestStub) DrawSprites(SpriteBatch) error        { return nil }
func (*driverTestStub) DrawShapes(ShapeBatch) error          { return nil }
func (*driverTestStub) Present() error                       { return nil }
func (*driverTestStub) ResizeSurface(int, int) error         { return nil }
func (*driverTestStub) SetVSync(bool)                        {}
func (*driverTestStub) Destroy()                             {}
