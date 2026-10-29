package scene

import (
	"errors"
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	onixtext "github.com/diiyw/onix/text"
	"github.com/go-gl/mathgl/mgl32"
)

func outputTestFont(t testing.TB) *onixtext.Font {
	t.Helper()
	data, err := os.ReadFile("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := onixtext.LoadFont(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Dispose(); err != nil {
			t.Error(err)
		}
	})
	return f
}

func TestRenderOutputLeasesAreIndependentAndRejectRecycledIdentity(t *testing.T) {
	f := outputTestFont(t)
	p := NewRenderOutputPool(2)
	released := 0
	h, err := p.Text(Text2D{Value: "Lease", Font: f, FontSize: 18, DPI: 72, Color: graphics.White, Alpha: 1}, func() error { released++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := h.Retain()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Retain(); !errors.Is(err, ErrOutputCapacity) {
		t.Fatal("lease capacity not enforced")
	}
	if err = h.Release(); err != nil {
		t.Fatal(err)
	}
	if h.Valid() || !consumer.Valid() || released != 0 {
		t.Fatal("provider lease release invalidated consumer")
	}
	second, err := consumer.Retain()
	if err != nil {
		t.Fatal(err)
	}
	if err = h.Release(); !errors.Is(err, ErrOutputReleased) || !second.Valid() {
		t.Fatal("old identity released a recycled lease")
	}
	if err = consumer.Release(); err != nil {
		t.Fatal(err)
	}
	if err = second.Release(); err != nil || released != 1 || p.Leases() != 0 {
		t.Fatal("last lease did not release output once")
	}
}

func TestRenderOutputFreezesLayoutAndComposesConsumerTransform(t *testing.T) {
	f := outputTestFont(t)
	value := Text2D{Value: "Output", Font: f, FontSize: 18, DPI: 72, Color: graphics.White, Alpha: 1, Spans: []TextSpan2D{{Start: 0, End: 1, Color: graphics.White}}}
	if !value.ensureLayout() {
		t.Fatal("layout failed")
	}
	p := NewRenderOutputPool(4)
	h, err := p.TextBox(value, Size2D{Width: 50, Height: 12}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	value.Value = "changed"
	value.Spans[0].Color = graphics.Black
	if !value.ensureLayout() || h.output().text.Value != "Output" || h.output().text.Spans[0].Color != graphics.White {
		t.Fatal("producer modified frozen output")
	}
	w, hgt, ok := h.Size()
	if !ok || w != 50 || hgt != 12 {
		t.Fatalf("bounds %v %v", w, hgt)
	}
	world := ecs.NewWorld()
	b := ecs.NewBundle(3)
	b.Put(RenderOutput2D{Handle: h})
	b.Put(UIGlobalTransform{Matrix: mgl32.Translate3D(123, 45, 0)})
	b.Put(Visibility{Visible: true})
	b.Put(AnchorTopLeft)
	e := world.Spawn(b)
	c := graphics.Canvas2DContext(320, 240)
	snapshots := renderSnapshots2D{}
	hierarchy := frameRenderHierarchy2DCache(world)
	items := snapshots.appendOutputRenderItems2D[UIGlobalTransform](c, world, &hierarchy, nil, false, true)
	if len(items) == 0 || items[0].entity != e || items[0].transform[12] != 123 || items[0].transform[13] != 45 {
		t.Fatalf("output did not use consumer coordinates: %+v", items)
	}
	if items[0].clip.minX != 123 || items[0].clip.maxX != 173 || items[0].clip.maxY != 57 {
		t.Fatalf("output clip %+v", items[0].clip)
	}
	step := func() {
		snapshots.reset()
		items = snapshots.appendOutputRenderItems2D[UIGlobalTransform](c, world, &hierarchy, items[:0], false, true)
	}
	for range 8 {
		step()
	}
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("stable output extraction: %v allocs", allocs)
	}
}
