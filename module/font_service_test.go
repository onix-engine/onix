package module

import (
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

type fontFixture struct {
	r         *Runtime
	providers []*Context
	clients   []Requester[TextRenderRequest, TextRenderResult]
	labels    []Slot
	errors    []error
}

func setupFonts(t testing.TB, options Options) *fontFixture {
	t.Helper()
	data, err := os.ReadFile("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	must(t, err)
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"fa","mod":"font"},{"id":"fb","mod":"font"},{"id":"a","mod":"client"},{"id":"b","mod":"client"}]}`,
		"font/mod.json":              `{"resources":{"regular":{"kind":"font","path":"private.ttf"}}}`,
		"font/private.ttf":           string(data),
		"client/mod.json":            `{"slots":{"label":{"type":"visual"}}}`,
	})
	f := &fontFixture{}
	registry := Registry{}
	must(t, registry.RegisterMod("font", func() FontService { return FontService{} }, Lifecycle[FontService]{Install: func(c *Context, s *FontService) error {
		f.providers = append(f.providers, c)
		if err := handleRequest(c, "ask", s.Render); err != nil {
			return err
		}
		return s.Load(c, "regular")
	}, Destroy: func(_ *Context, s *FontService) error { return s.Destroy() }}))
	must(t, registry.RegisterMod("client", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		label, err := c.Slot("label")
		if err != nil {
			return err
		}
		f.labels = append(f.labels, label)
		client, err := testRequester[TextRenderRequest, TextRenderResult](c, "ask", func(_ *Context, value Result[TextRenderResult]) error {
			if value.Err != nil {
				f.errors = append(f.errors, value.Err)
				return nil
			}
			w, h, ok := value.Value.Output.Size()
			if !ok || w != value.Value.Width || h != value.Value.Height {
				return ErrType
			}
			return label.SetVisual(value.Value.Output)
		})
		f.clients = append(f.clients, client)
		return err
	}}))
	options.Source = source
	f.r = newFixture(t, plan, registry, options)
	_, err = f.r.Manager().StartEntry()
	must(t, err)
	return f
}
func fontRequest(text string) TextRenderRequest {
	return TextRenderRequest{Text: text, Size: 18, Color: [4]float32{1, 1, 1, 1}, Width: 200, Height: 40}
}

func TestFontServiceSharesBoundedCacheAndKeepsLeasedOutputsAfterProvidersDie(t *testing.T) {
	f := setupFonts(t, Options{TextCacheCapacity: 2})
	for _, client := range f.clients {
		_, err := client.SendTo("f"+client.context.name, fontRequest("shared"), RequestOptions{})
		must(t, err)
	}
	must(t, f.r.Dispatch())
	must(t, f.r.Dispatch())
	if len(f.r.fonts) != 1 || f.r.outputs.Leases() != 3 {
		t.Fatal("identical font/content did not share output")
	}
	for i := range 10 {
		_, err := f.clients[0].SendTo("fa", fontRequest(strconv.Itoa(i)), RequestOptions{})
		must(t, err)
		must(t, f.r.Dispatch())
		must(t, f.r.Dispatch())
	}
	for _, resource := range f.r.fonts {
		if len(resource.outputs) != 2 || resource.providers != 2 {
			t.Fatal("font cache was not bounded or provider state was shared")
		}
	}
	for _, provider := range f.providers {
		must(t, provider.Dispose())
	}
	if len(f.r.fonts) != 1 || f.r.outputs.Leases() != 2 {
		t.Fatal("provider destruction invalidated live outputs")
	}
	for _, label := range f.labels {
		h, _ := label.Visual()
		if !h.Valid() {
			t.Fatal("output was released early")
		}
		must(t, label.SetVisual(RenderOutputHandle{}))
	}
	if len(f.r.fonts) != 0 || f.r.outputs.Leases() != 0 {
		t.Fatal("last consumers leaked font/atlas ownership")
	}
}

func TestFontServiceLeaseOverflowAndInvalidRequestReportFailure(t *testing.T) {
	f := setupFonts(t, Options{OutputCapacity: 1})
	_, err := f.clients[0].SendTo("fa", fontRequest("text"), RequestOptions{})
	must(t, err)
	must(t, f.r.Dispatch())
	must(t, f.r.Dispatch())
	if len(f.errors) != 1 || !errors.Is(f.errors[0], scene.ErrOutputCapacity) {
		t.Fatal("lease overflow was not reported")
	}
	request := fontRequest("invalid")
	request.Size = -1
	_, err = f.clients[0].SendTo("fa", request, RequestOptions{})
	must(t, err)
	must(t, f.r.Dispatch())
	must(t, f.r.Dispatch())
	if len(f.errors) != 2 || !errors.Is(f.errors[1], ErrInvalid) {
		t.Fatal("invalid request did not fail")
	}
}

func TestCachedFontRequestAllocatesNothingWarm(t *testing.T) {
	f := setupFonts(t, Options{})
	step := func() {
		if _, err := f.clients[0].SendTo("fa", fontRequest("cached"), RequestOptions{}); err != nil {
			panic(err)
		}
		if err := f.r.Dispatch(); err != nil {
			panic(err)
		}
		if err := f.r.Dispatch(); err != nil {
			panic(err)
		}
	}
	for range 8 {
		step()
	}
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("cached font requests: %v allocs", allocs)
	}
}

func BenchmarkCachedFontRequest(b *testing.B) {
	f := setupFonts(b, Options{})
	_, err := f.clients[0].SendTo("fa", fontRequest("cached"), RequestOptions{})
	must(b, err)
	must(b, f.r.Dispatch())
	must(b, f.r.Dispatch())
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err = f.clients[0].SendTo("fa", fontRequest("cached"), RequestOptions{})
		if err != nil {
			b.Fatal(err)
		}
		must(b, f.r.Dispatch())
		must(b, f.r.Dispatch())
	}
}

func TestCombinedSlotRevisionRejectsAdditionalSlotChanges(t *testing.T) {
	f := visualTestFixture(t, true)
	// The first guard is unchanged; changing the second must still discard.
	_, err := f.requester.SendTo("provider", "text", RequestOptions{Revision: f.label.Revision().With(f.text.Revision())})
	must(t, err)
	must(t, f.r.Dispatch())
	f.reply(t)
	must(t, f.text.Set(project.SlotValue{Type: project.StringValue, String: "new"}))
	must(t, f.r.Dispatch())
	if !errors.Is(f.lastError, ErrStaleRevision) || f.pool.Leases() != 1 {
		t.Fatal("combined revision accepted stale result")
	}
}
