package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestHTMLSyncStateDiagnostics(t *testing.T) {
	w, e := incrementalHTMLFixture(t, `<div id="a" style="background:red;width:10px;height:10px"></div>`)
	defer Dispose(w)
	check := func(path SyncPath, reason FallbackReason) {
		t.Helper()
		if got := Diagnostics(w, e); got.Path != path || got.Fallback != reason {
			t.Fatalf("diagnostics=%+v want %s/%s", got, path, reason)
		}
	}
	check(SyncFull, FallbackInitial)
	n := incrementalHTMLNode(t, w, e, "a")
	n.SetAttribute("style", "background:blue;width:10px;height:10px")
	if err := Sync(w); err != nil {
		t.Fatal(err)
	}
	check(SyncStyle, FallbackNone)
	n.SetAttribute("style", "background:blue;width:20px;height:10px")
	if err := Sync(w); err != nil {
		t.Fatal(err)
	}
	check(SyncPartial, FallbackNone)
	if err := Sync(w); err != nil {
		t.Fatal(err)
	}
	check(SyncCached, FallbackNone)
	ecs.GetMut[Surface](w, e).Value = `<div id="b">replacement</div>`
	if err := Sync(w); err != nil {
		t.Fatal(err)
	}
	check(SyncFull, FallbackDocument)
	state := ecs.GetMut[htmlRenderState](w, e)
	state.requestLayout()
	if !NeedsFrame(w) {
		t.Fatal("pending layout with unchanged render key was lost")
	}
	if err := Sync(w); err != nil {
		t.Fatal(err)
	}
	check(SyncFull, FallbackLayout)
	if state = ecs.GetRef[htmlRenderState](w, e); state.pending() || len(state.Mutations) != 0 {
		t.Fatal("completed sync retained pending changes")
	}
	if n := testing.AllocsPerRun(100, func() { _ = Diagnostics(w, e) }); n != 0 {
		t.Fatalf("diagnostics allocated %v", n)
	}
}
func TestHTMLPartialFallbackDiagnostics(t *testing.T) {
	w, e := incrementalHTMLFixture(t, `<div id="a" style="background:red;width:10px;height:10px"></div>`)
	defer Dispose(w)
	n := incrementalHTMLNode(t, w, e, "a")
	n.SetAttribute("style", "background:blue;width:20px;height:10px")
	ecs.GetMut[htmlRenderState](w, e).PartialValid = false
	if err := Sync(w); err != nil {
		t.Fatal(err)
	}
	if got := Diagnostics(w, e); got.Path != SyncFull || got.Fallback != FallbackRetained {
		t.Fatalf("fallback=%+v", got)
	}
}

func TestHTMLStyleSyncKeepsDeferredInputPaint(t *testing.T) {
	w, e := incrementalHTMLFixture(t, `<div id="a" style="background:red;width:10px;height:10px"></div>`)
	defer Dispose(w)
	// Pointer-state changes can defer paint until an active drag finishes.
	ecs.GetMut[htmlRenderState](w, e).LayoutDirty = true
	incrementalHTMLNode(t, w, e, "a").SetAttribute("style", "background:blue;width:10px;height:10px")
	if err := Sync(w); err != nil {
		t.Fatal(err)
	}
	state := ecs.GetRef[htmlRenderState](w, e)
	if Diagnostics(w, e).Path != SyncStyle || !state.LayoutDirty {
		t.Fatal("style phase consumed the deferred input paint")
	}
}
