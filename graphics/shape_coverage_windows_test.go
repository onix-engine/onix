//go:build windows && cgo

package graphics

import (
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
	"golang.org/x/sys/windows"
)

// Read an offscreen target using a hidden native window; no editor or visual
// acceptance session is needed to exercise the actual shape shader.
func TestShapeCoverageVulkanPixels(t *testing.T) {
	for _, test := range []struct {
		name      string
		density   int
		projected bool
	}{{"normal", 1, false}, {"hidpi", 2, false}, {"camera", 1, true}} {
		t.Run(test.name, func(t *testing.T) { testShapeCoverageVulkanPixels(t, test.density, test.projected) })
	}
}

func testShapeCoverageVulkanPixels(t *testing.T, density int, projected bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := windows.NewLazySystemDLL("user32.dll")
	class, _ := windows.UTF16PtrFromString("STATIC")
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 64, 64, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal("create hidden window:", err)
	}
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	driver, err := render.NewVulkanDriver(render.VulkanSurface{Kind: render.VulkanSurfaceWin32, Window: hwnd, Width: 64, Height: 64})
	if err != nil {
		t.Skip("Vulkan unavailable:", err)
	}
	defer driver.Destroy()
	device, err := render.NewDevice(driver)
	if err != nil {
		t.Fatal(err)
	}
	c := NewCanvas2D(device, 256, 256)
	if projected {
		c.SetProjectionMatrix(mgl32.Ortho2D(0, 256, 256, 0))
	}
	size := 256 * density
	targets := driver.(render.RenderTargetDriver)
	readback := driver.(render.RenderTargetReadbackDriver)
	handle, _, err := targets.CreateRenderTarget(render.RenderTargetDescriptor{Width: size, Height: size})
	if err != nil {
		t.Fatal(err)
	}
	defer targets.DestroyRenderTarget(handle)
	pixels := make([]byte, size*size*4)
	for _, degrees := range []float32{0, 5, -5, 45} {
		for _, scale := range []float32{.65, 1.1} {
			angle := degrees * math.Pi / 180
			base := mgl32.Translate3D(64.25, 32.25, 0).Mul4(mgl32.HomogRotate3DZ(angle)).Mul4(mgl32.Scale3D(scale, scale, 1))
			c.ClearColor(ColorRGBA{})
			if err := targets.BeginRenderTarget(handle, render.RenderTargetClear{ClearColor: true}); err != nil {
				t.Fatal(err)
			}
			driver.SetViewport(render.Rect{Width: int32(size), Height: int32(size)})
			driver.SetScissor(nil)
			c.DrawShapeMatrices(base.Mul4(mgl32.Scale3D(170, 44, 1)), mgl32.Ident3(), 170, 44, render.ShapeRoundedRect, White, ColorRGBA{}, 4, 0, .65)
			c.DrawFrame()
			if err := targets.EndRenderTarget(); err != nil {
				t.Fatal(err)
			}
			driver.SetViewport(render.Rect{Width: 64, Height: 64})
			if err := c.FinishFrame(); err != nil {
				t.Fatal(err)
			}
			if err := driver.Present(); err != nil {
				t.Fatal(err)
			}
			if err := readback.ReadRenderTarget(handle, render.RenderTargetRegion{Width: size, Height: size}, pixels); err != nil {
				t.Fatal(err)
			}
			inverse := base.Inv()
			sin, cos := math.Sincos(float64(angle))
			footprint := (math.Abs(sin) + math.Abs(cos)) / (float64(scale) * float64(density))
			outside := 0
			for y := range size {
				for x := range size {
					p := inverse.Mul4x1(mgl32.Vec4{(float32(x) + .5) / float32(density), (float32(y) + .5) / float32(density), 0, 1})
					var distance float64
					switch {
					case p[0] > 10 && p[0] < 160:
						distance = math.Max(-float64(p[1]), float64(p[1])-44)
					case p[1] > 10 && p[1] < 34:
						distance = math.Max(-float64(p[0]), float64(p[0])-170)
					default:
						continue
					}
					if math.Abs(distance) > footprint*.45 {
						continue
					}
					want := (.5 - distance/footprint) * .65 * 255
					got := pixels[(y*size+x)*4+3]
					if math.Abs(float64(got)-want) > 2 {
						t.Fatalf("angle=%g scale=%g pixel=(%d,%d) distance=%.3f alpha=%d, want %.1f", degrees, scale, x, y, distance, got, want)
					}
					if distance > 0 {
						outside++
					}
				}
			}
			if degrees != 0 && outside < 10 {
				t.Fatalf("angle=%g scale=%g: only %d exterior edge pixels checked", degrees, scale, outside)
			}
		}
	}
}
