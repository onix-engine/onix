package layout

import "testing"

func TestRootViewportStorageAcrossRetainedResize(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div><p>Text</p></div>`)
	t.Cleanup(func() { _ = face.Close() })
	var cache Cache
	for _, width := range []float64{320, 640, 240, 320} {
		box, err := cache.Update(root, styles, face, face, width, 200, nil, nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := BuildWithFonts(root, styles, face, face, width, 200)
		if err != nil {
			t.Fatal(err)
		}
		if box.Viewport() != (Rect{Width: width, Height: 200}) || fresh.Viewport() != box.Viewport() || box.GeometryHash() != fresh.GeometryHash() {
			t.Fatal("viewport or geometry hash differs after retained resize")
		}
		assertRetainedMatchesFull(t, box, fresh)
		for node, child := range cache.nodes {
			if node != root && (child.viewport != nil || child.Viewport() != (Rect{})) {
				t.Fatal("descendant retained document viewport storage")
			}
		}
	}
}
