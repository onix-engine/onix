package module

import (
	"errors"
	"os"
	"testing"

	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
)

type visualFixture struct {
	r                    *Runtime
	s                    *Scene
	client, provider     *Context
	requester            Requester[string, RenderOutputHandle]
	request              Request[string, RenderOutputHandle]
	label, text          Slot
	pool                 *scene.RenderOutputPool
	output               RenderOutputHandle
	destroyed, callbacks int
	lastError            error
}

func visualTestFixture(t testing.TB, bind bool) *visualFixture {
	t.Helper()
	f := &visualFixture{pool: scene.NewRenderOutputPool(8)}
	data, err := os.ReadFile("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	must(t, err)
	font, err := onixtext.LoadFont(data)
	must(t, err)
	f.output, err = f.pool.Text(scene.Text2D{Value: "Lease", Font: font, FontSize: 16, DPI: 72, Alpha: 1}, func() error { f.destroyed++; return font.Dispose() })
	must(t, err)
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"client","mod":"client"},{"id":"provider","mod":"provider"}]}`,
		"client/mod.json":            `{"slots":{"text":{"type":"string"},"label":{"type":"visual","placement":{"position":[5,7]}}}}`,
		"provider/mod.json":          `{}`,
	})
	registry := Registry{}
	must(t, registry.RegisterMod("client", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		f.client = c
		f.label, _ = c.Slot("label")
		f.text, _ = c.Slot("text")
		var callback func(*Context, Result[RenderOutputHandle]) error
		if bind {
			callback = func(_ *Context, value Result[RenderOutputHandle]) error {
				f.callbacks++
				f.lastError = value.Err
				if value.Err == nil {
					return f.label.SetVisual(value.Value)
				}
				return nil
			}
		}
		var err error
		f.requester, err = testRequester[string, RenderOutputHandle](c, "ask", callback)
		return err
	}}))
	must(t, registry.RegisterMod("provider", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		f.provider = c
		return handleRequest(c, "ask", func(_ *Context, q Request[string, RenderOutputHandle]) error { f.request = q; return nil })
	}, Destroy: func(*Context, *Signal) error { return f.output.Release() }}))
	f.r = newFixture(t, plan, registry, Options{Source: source})
	f.s, err = f.r.MountEntry()
	must(t, err)
	return f
}
func (f *visualFixture) reply(t testing.TB) {
	t.Helper()
	lease, err := f.output.Retain()
	must(t, err)
	if err = f.request.Reply(f.provider, lease); err != nil {
		_ = lease.Release()
		t.Fatal(err)
	}
}

func TestVisualReplySurvivesProviderDisposalAndSlotOwnsLease(t *testing.T) {
	f := visualTestFixture(t, true)
	_, err := f.requester.SendTo("provider", "text", RequestOptions{})
	must(t, err)
	must(t, f.r.Dispatch())
	f.reply(t)
	must(t, f.provider.Dispose())
	if f.destroyed != 0 {
		t.Fatal("queued output died with provider")
	}
	must(t, f.r.Dispatch())
	h, ok := f.label.Visual()
	if !ok || !h.Valid() || f.callbacks != 1 || f.pool.Leases() != 1 {
		t.Fatal("slot did not consume reply")
	}
	if _, err = f.requester.SendTo("provider", "again", RequestOptions{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("disposed provider accepted request")
	}
	must(t, f.label.SetVisual(RenderOutputHandle{}))
	if f.destroyed != 1 || f.pool.Leases() != 0 {
		t.Fatal("clear did not release final lease")
	}
}

func TestVisualDiscardReleasesCanceledStaleUnhandledAndClosedReplies(t *testing.T) {
	for _, mode := range []string{"canceled", "stale", "unhandled", "close", "source"} {
		t.Run(mode, func(t *testing.T) {
			f := visualTestFixture(t, mode != "unhandled")
			id, err := f.requester.SendTo("provider", "text", RequestOptions{Revision: f.text.Revision()})
			must(t, err)
			must(t, f.r.Dispatch())
			f.reply(t)
			switch mode {
			case "canceled":
				must(t, f.requester.Cancel(id))
			case "stale":
				must(t, f.text.Set(project.SlotValue{Type: project.StringValue, String: "new"}))
			case "close":
				must(t, f.r.Close())
			case "source":
				must(t, f.client.Dispose())
			}
			if mode != "close" {
				must(t, f.r.Dispatch())
				if f.pool.Leases() != 1 {
					t.Fatalf("discard leaked leases: %d", f.pool.Leases())
				}
				must(t, f.provider.Dispose())
			}
			if f.pool.Leases() != 0 || f.destroyed != 1 {
				t.Fatal("output resource leaked")
			}
			if mode == "stale" && !errors.Is(f.lastError, ErrStaleRevision) {
				t.Fatal("stale reply was delivered")
			}
		})
	}
}

func TestVisualReplaceRetainAndDispatchAllocateNothingWarm(t *testing.T) {
	f := visualTestFixture(t, true)
	step := func() {
		if _, err := f.requester.SendTo("provider", "cached", RequestOptions{}); err != nil {
			panic(err)
		}
		if err := f.r.Dispatch(); err != nil {
			panic(err)
		}
		lease, err := f.output.Retain()
		if err != nil {
			panic(err)
		}
		if err = f.request.Reply(f.provider, lease); err != nil {
			panic(err)
		}
		if err = f.r.Dispatch(); err != nil {
			panic(err)
		}
	}
	for range 8 {
		step()
	}
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("visual request and replacement: %v allocs", allocs)
	}
	if f.pool.Leases() != 2 {
		t.Fatal("replacement leaked prior slot output")
	}
}

func BenchmarkVisualReply(b *testing.B) {
	f := visualTestFixture(b, true)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err := f.requester.SendTo("provider", "cached", RequestOptions{})
		if err != nil {
			b.Fatal(err)
		}
		must(b, f.r.Dispatch())
		f.reply(b)
		must(b, f.r.Dispatch())
	}
}

func TestFailedMountReleasesTransferredVisualPayload(t *testing.T) {
	f := visualTestFixture(t, true)
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"fail","mod":"fail"}]}`,
		"fail/mod.json":              `{}`,
	})
	registry := Registry{}
	must(t, registry.RegisterMod("fail", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		emit, err := c.BindEmitter[RenderOutputHandle]("output")
		if err != nil {
			return err
		}
		lease, err := f.output.Retain()
		if err != nil {
			return err
		}
		if err = emit.Emit(lease); err != nil {
			_ = lease.Release()
			return err
		}
		return ErrInvalid
	}}))
	r := newFixture(t, plan, registry, Options{})
	_, err := r.MountEntry()
	if !errors.Is(err, ErrInvalid) || f.pool.Leases() != 1 || r.Queued() != 0 {
		t.Fatal("failed mount leaked transferred output")
	}
}

func TestModuleForwardRetainsVisualUntilNextDelivery(t *testing.T) {
	f := visualTestFixture(t, false)
	var delivered int
	f.client.message = func(m Message) error { return f.client.Forward("provider", m) }
	f.provider.message = func(m Message) error {
		output, ok := m.Value[RenderOutputHandle]()
		if !ok || !output.Valid() {
			t.Fatal("forwarded lease expired")
		}
		delivered++
		return nil
	}
	lease, err := f.output.Retain()
	must(t, err)
	must(t, f.provider.Send("client", "output", lease))
	must(t, f.r.Dispatch())
	if delivered != 0 || f.pool.Leases() != 2 {
		t.Fatal("queued relay lost its lease", delivered, f.pool.Leases())
	}
	must(t, f.r.Dispatch())
	if delivered != 1 || f.pool.Leases() != 1 {
		t.Fatal("delivered relay leaked its lease", delivered, f.pool.Leases())
	}
	must(t, f.provider.Dispose())
	if f.destroyed != 1 {
		t.Fatal("forwarded visual resource leaked")
	}
}
