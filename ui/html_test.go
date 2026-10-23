package ui

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLInputEditsAndEmitsEvents(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)

	html := htmlui.New(`<div><input id="name" name="display_name" value="Onix" maxlength="8"><button id="send">Send</button></div>`)
	html.Width, html.Height = 320, 100
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(5)
	ecs.Put(&bundle, scene.NewUITransform())
	ecs.Put(&bundle, scene.UIGlobalTransform{})
	ecs.Put(&bundle, scene.Visibility{Visible: true})
	ecs.Put(&bundle, scene.AnchorTopLeft)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "name")
	button := htmlControlByID(t, world, owner, "send")
	textField, ok := ecs.Get[TextField](world, field)
	if !ok || textField.Value != "Onix" || textField.MaxRunes != 8 || !ecs.Has[Input](world, field) {
		t.Fatalf("HTML input was not bound to TextField: %#v, ok=%v", textField, ok)
	}
	if !ecs.Has[Button](world, button) {
		t.Fatal("HTML button was not bound to Button")
	}

	reader := &ecs.MessageReader[htmlui.Event]{}
	ecs.WriteMessage(world, scene.InputEvent{Entity: field, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	assertHTMLEvent(t, reader.Read(world), owner, "name", htmlui.EventFocus, "Onix")
	textField, _ = ecs.Get[TextField](world, field)
	textField.Caret, textField.Anchor = 3, 3
	if current := ecs.GetMut[TextField](world, field); current != nil {
		*current = textField
	}

	uiInputState.EndFrame()
	uiInputState.CommitText("!")
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	assertHTMLEvent(t, reader.Read(world), owner, "name", htmlui.EventInput, "Onix!")
	control, _ := ecs.Get[htmlui.Control](world, field)
	if control.Value != "Onix!" {
		t.Fatalf("HTML control value = %q, want Onix!", control.Value)
	}

	uiInputState.EndFrame()
	uiInputState.PressKey(input.KeyEnter)
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	events := reader.Read(world)
	assertHTMLEvent(t, events, owner, "name", htmlui.EventChange, "Onix!")
	assertHTMLEvent(t, events, owner, "name", htmlui.EventSubmit, "Onix!")

	uiInputState.EndFrame()
	ecs.WriteMessage(world, scene.InputEvent{Entity: button, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	events = reader.Read(world)
	assertHTMLEvent(t, events, owner, "send", htmlui.EventClick, "")
	assertHTMLEvent(t, events, owner, "send", htmlui.EventSubmit, "")

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	foundValue := false
	for _, access := range ecs.Query2[Relation[scene.Text2D], scene.Text2D](world).All {
		relation := access.A.Get()
		text := access.B.Get()

		if relation.Owner == field && text.Value == "Onix!" {
			foundValue = true
		}
	}
	if !foundValue {
		t.Fatal("edited HTML input value was not synchronized to its text visual")
	}
	foundCaret := false
	for _, access := range ecs.Query2[htmlui.ControlVisual, scene.Visibility](world).All {
		visual := access.A.Get()
		visibility := access.B.Get()

		if visual.Control == field && !visual.Selection {
			foundCaret = visibility.Visible
			break
		}
	}
	if !foundCaret {
		t.Fatal("focused HTML input did not display its generated caret")
	}
}

func TestHTMLOverflowScrollMovesContentAfterLayout(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div id="viewport" style="width:100px;height:40px;overflow:auto"><div style="width:240px;height:80px;background:#fff"></div></div>`)
	html.Width, html.Height = 300, 100
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	var area ecs.Entity
	for entity, access := range ecs.Query2[htmlui.ScrollContent, scene.UITransform](world).All {
		content := access.A.Get()

		if content.Area != 0 {
			area = content.Area
			_ = entity
			break
		}
	}
	if area == 0 {
		t.Fatal("missing generated HTML scroll content")
	}
	scroll := ecs.GetMut[scene.UIScroll](world, area)
	if scroll == nil {
		t.Fatal("missing generated HTML scroll state")
	}
	var contentEntity ecs.Entity
	var before float32
	for entity, access := range ecs.Query2[htmlui.ScrollContent, scene.UITransform](world).All {
		content := access.A.Get()
		transform := access.B.Get()

		if content.Area == area {
			contentEntity, before = entity, transform.Translation.X.Value
			break
		}
	}
	scroll.Position[0] = -30
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	transform, _ := ecs.Get[scene.UITransform](world, contentEntity)
	if transform.Translation.X.Value != before-30 {
		t.Fatalf("scroll content x = %v, want %v", transform.Translation.X.Value, before-30)
	}
}

func TestHTMLTextControlSelectionSurvivesDOMUpdateAndSourceReplacement(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		id        string
		anchor    int
		caret     int
		multiline bool
		reparse   bool
	}{
		{name: "input", html: `<input id="field" value="selection">`, id: "field", anchor: 2, caret: 7},
		{name: "textarea", html: `<textarea id="field">first line&#10;second line</textarea>`, id: "field", anchor: 15, caret: 3, multiline: true},
		{name: "source update", html: `<input id="field" value="selection">`, id: "field", anchor: 8, caret: 1, reparse: true},
		{name: "unnamed input", html: `<input value="selection">`, anchor: 2, caret: 7, reparse: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorld()
			RegisterRuntimeFeature[HTMLRenderer](world)
			uiInputState := input.NewState()
			ecs.InsertResource(world, uiInputState)
			html := htmlui.New(test.html)
			html.Width, html.Height = 320, 100
			html.Font = notosanssc.NotoSansSCRegular
			bundle := ecs.NewBundle(1)
			ecs.Put(&bundle, html)
			owner := world.Spawn(bundle)
			sync := SyncSystem("")
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			oldEntity := htmlControlByID(t, world, owner, test.id)
			input := &uiEventState{}
			ecs.InsertResource(world, input)
			focus(world, input, oldEntity)
			field := ecs.GetMut[TextField](world, oldEntity)
			field.Anchor, field.Caret, field.Selecting = test.anchor, test.caret, true
			selectionStart, selectionEnd := orderedSelection(test.anchor, test.caret)
			uiInputState.SetTextInputSelection(selectionStart, selectionEnd)
			capture := &scene.InputCapture2D{Target: oldEntity, Receiver: oldEntity}
			ecs.InsertResource(world, capture)

			if test.reparse {
				if !htmlui.Set(world, owner, test.html+`<style>input{width:180px}</style>`) {
					t.Fatal("update HTML source")
				}
			} else {
				node, ok := htmlui.NodeByID(world, owner, test.id)
				if !ok || !node.SetAttribute("data-rebuild", "1") {
					t.Fatal("mark HTML text control dirty")
				}
			}
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			newEntity := htmlControlByID(t, world, owner, test.id)
			if test.reparse {
				if newEntity == oldEntity || world.IsAlive(oldEntity) {
					t.Fatalf("source replacement retained old node: old=%d new=%d", oldEntity, newEntity)
				}
			} else if newEntity != oldEntity || !world.IsAlive(oldEntity) {
				t.Fatalf("DOM update replaced control: old=%d new=%d", oldEntity, newEntity)
			}
			field = ecs.GetMut[TextField](world, newEntity)
			if !field.Focused || !field.Selecting || field.Anchor != test.anchor || field.Caret != test.caret {
				t.Fatalf("rebuilt text field state = %#v", *field)
			}
			if test.multiline {
				if !ecs.Has[Textarea](world, newEntity) {
					t.Fatal("rebuilt textarea lost its textarea component")
				}
			}
			if input.Focused != newEntity {
				t.Fatalf("input focus = %d, want %d", input.Focused, newEntity)
			}
			keyboard, _ := ecs.GetResource[*scene.InputFocus](world)
			if keyboard == nil || keyboard.Keyboard != newEntity {
				t.Fatalf("keyboard focus = %#v, want %d", keyboard, newEntity)
			}
			if capture.Target != newEntity || capture.Receiver != newEntity {
				t.Fatalf("input capture = %#v, want %d", capture, newEntity)
			}
			config := uiInputState.TextInputConfig()
			selectionStart, selectionEnd = orderedSelection(test.anchor, test.caret)
			if config.SelectionStart != selectionStart || config.SelectionEnd != selectionEnd {
				t.Fatalf("platform selection = %d..%d, want %d..%d", config.SelectionStart, config.SelectionEnd, selectionStart, selectionEnd)
			}
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			selectionVisible := false
			for _, access := range ecs.Query2[htmlui.ControlVisual, scene.Visibility](world).All {
				visual := access.A.Get()
				visibility := access.B.Get()

				if visual.Control == newEntity && visual.Selection && visibility.Visible {
					selectionVisible = true
					break
				}
			}
			if !selectionVisible {
				t.Fatal("rebuilt HTML text control did not render its selection")
			}
		})
	}
}

func TestHTMLTextareaSelectionPaintsEveryRowThroughTheCaret(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	value := strings.Repeat("word\n", 24) + "tail"
	html := htmlui.New("<textarea id=\"field\">" + value + "</textarea><style>textarea{width:640px;height:600px;font-size:13px;line-height:20px;white-space:pre}</style>")
	html.Width, html.Height = 700, 700
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	input := &uiEventState{}
	ecs.InsertResource(world, input)
	focus(world, input, field)
	if fieldState := ecs.GetMut[TextField](world, field); fieldState != nil {
		fieldState.Anchor, fieldState.Caret, fieldState.Selecting = 0, runeCount(value), true
	}
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	painted := 0
	for _, access := range ecs.Query3[htmlui.ControlVisual, scene.Visibility, scene.Size2D](world).All {
		visual, visibility, size := access.A.Get(), access.B.Get(), access.C.Get()

		if visual.Control == field && visual.Selection && visibility.Visible && size.Width > 0 {
			painted++
		}
	}
	if painted != 25 {
		t.Fatalf("painted selection rows = %d, want one per selected row of 25", painted)
	}
}

func TestHTMLTextareaSelectionPaintsWrappedRows(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	value := strings.Repeat("word ", 20) + "end"
	html := htmlui.New("<textarea id=\"field\">" + value + "</textarea><style>textarea{width:120px;height:300px;font-size:13px;line-height:20px;white-space:pre-wrap}</style>")
	html.Width, html.Height = 200, 400
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	input := &uiEventState{}
	ecs.InsertResource(world, input)
	focus(world, input, field)
	if fieldState := ecs.GetMut[TextField](world, field); fieldState != nil {
		fieldState.Anchor, fieldState.Caret, fieldState.Selecting = 0, runeCount(value), true
	}
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	textEntity, ok := FindPart[ValuePart, scene.Text2D](world, field)
	if !ok {
		t.Fatal("textarea value visual missing")
	}
	valueText := ecs.GetRef[scene.Text2D](world, textEntity)
	textTransform, _ := ecs.Get[scene.UITransform](world, textEntity)
	rows := scene.TextLineCount2D(valueText)
	painted, lowest := 0, float32(0)
	for entity, access := range ecs.Query3[htmlui.ControlVisual, scene.Visibility, scene.Size2D](world).All {
		visual, visibility, size := access.A.Get(), access.B.Get(), access.C.Get()

		if visual.Control != field || !visual.Selection || !visibility.Visible || size.Width <= 0 {
			continue
		}
		painted++
		transform, _ := ecs.Get[scene.UITransform](world, entity)
		lowest = max(lowest, transform.Translation.Y.Value+size.Height)
	}
	if rows < 2 {
		t.Fatalf("wrapped textarea laid out %d rows, want the source line to wrap", rows)
	}
	if painted != rows {
		t.Fatalf("painted selection rows = %d, want one per wrapped row of %d", painted, rows)
	}
	_, caretY, _, caretHeight, caretOK := scene.TextCaretRect2D(valueText, value, runeCount(value))
	if !caretOK {
		t.Fatal("caret rect missing")
	}
	if lowest < textTransform.Translation.Y.Value+caretY+caretHeight {
		t.Fatalf("selection bottom = %v, want it to reach the caret row at %v", lowest, textTransform.Translation.Y.Value+caretY+caretHeight)
	}
}

func TestHTMLTextareaPaintsSelectionBehindCaretAndValueText(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	value := strings.Repeat("word\n", 4) + "tail"
	html := htmlui.New("<textarea id=\"field\">" + value + "</textarea><style>textarea{width:320px;height:160px;font-size:13px;line-height:20px;white-space:pre}</style>")
	html.Width, html.Height = 400, 240
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	input := &uiEventState{}
	ecs.InsertResource(world, input)
	focus(world, input, field)
	if fieldState := ecs.GetMut[TextField](world, field); fieldState != nil {
		fieldState.Anchor, fieldState.Caret, fieldState.Selecting = 0, runeCount(value), true
	}
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	textEntity, ok := FindPart[ValuePart, scene.Text2D](world, field)
	if !ok {
		t.Fatal("textarea value visual missing")
	}
	textStack, ok := ecs.Get[scene.UIStackIndex](world, textEntity)
	if !ok {
		t.Fatal("textarea value visual has no paint stack")
	}
	var selectionTop scene.UIStackIndex
	overlayBottom := scene.UIStackIndex(1 << 30)
	overlays := 0
	for _, access := range ecs.Query2[htmlui.ControlVisual, scene.UIStackIndex](world).All {
		visual, stack := access.A.Get(), access.B.Get()

		if visual.Control != field {
			continue
		}
		if visual.Selection {
			selectionTop = max(selectionTop, stack)
			continue
		}
		overlays++
		overlayBottom = min(overlayBottom, stack)
	}
	if overlays == 0 {
		t.Fatal("textarea caret and scrollbar visuals missing")
	}
	if selectionTop >= textStack {
		t.Fatalf("selection stack = %d, want it below value text stack %d", selectionTop, textStack)
	}
	if overlayBottom <= textStack {
		t.Fatalf("caret stack = %d, want it above value text stack %d", overlayBottom, textStack)
	}
}

func TestHTMLTextareaShowsStyledVerticalScrollbarWhenContentOverflows(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<textarea id="notes">line 01
line 02
line 03
line 04
line 05
line 06
line 07
line 08
line 09
line 10</textarea><style>textarea{width:120px;height:64px;padding:8px;overflow-x:hidden;overflow-y:auto;scrollbar-width:thin;scrollbar-color:#3a706f #0c1117}</style>`)
	html.Width, html.Height = 180, 100
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	for range 2 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}

	textarea := htmlControlByID(t, world, owner, "notes")
	state, ok := ecs.Get[Textarea](world, textarea)
	if !ok || state.BarSize != 10 {
		t.Fatalf("HTML textarea state = %#v, ok=%v", state, ok)
	}
	controlSize, _ := ecs.Get[scene.Size2D](world, textarea)
	verticalTrack, verticalThumb, horizontalTrack := false, false, false
	for entity, access := range ecs.Query3[Relation[SurfaceStyle], scene.UITransform, scene.Size2D](world).All {
		relation := access.A.Mut()
		transform := access.B.Mut()
		size := access.C.Mut()

		if relation.Owner != textarea {
			continue
		}
		background, _ := ecs.Get[scene.UIBackground2D](world, entity)
		switch {
		case ecs.Has[Part[ScrollTrackVerticalPart]](world, entity):
			verticalTrack = size.Width == 10 && size.Height > 0 && transform.Translation.X.Value+size.Width <= controlSize.Width
			if background.Shape.Fill != graphics.ColorFromRGBA(12, 17, 23, 255) {
				t.Fatalf("vertical scrollbar track color = %v", background.Shape.Fill)
			}
		case ecs.Has[Part[ScrollThumbVerticalPart]](world, entity):
			verticalThumb = size.Width > 0 && size.Height > 0
			if background.Shape.Fill != graphics.ColorFromRGBA(58, 112, 111, 255) {
				t.Fatalf("vertical scrollbar thumb color = %v", background.Shape.Fill)
			}
		case ecs.Has[Part[ScrollTrackHorizontalPart]](world, entity):
			horizontalTrack = size.Width > 0 && size.Height > 0
		}
	}
	if !verticalTrack || !verticalThumb || horizontalTrack {
		t.Fatalf("HTML textarea scrollbars = vertical track:%v thumb:%v horizontal:%v", verticalTrack, verticalThumb, horizontalTrack)
	}
}

func TestHTMLTextareaTransparentScrollbarColors(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	const baseStyle = "width:120px;height:64px;overflow-x:hidden;overflow-y:auto;scrollbar-width:thin;"
	const transparentTrack = "scrollbar-color:#65708280 transparent;scrollbar-thumb-hover-color:transparent"
	html := htmlui.New(`<textarea id="notes" style="` + baseStyle + transparentTrack + `">` + strings.Repeat("Line\n", 20) + `</textarea>`)
	html.Width, html.Height = 180, 100
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	defaults := skinFor(world, Theme{}).Scroll
	for _, test := range []struct {
		css   string
		track graphics.ColorRGBA
		thumb graphics.ColorRGBA
	}{
		{transparentTrack, graphics.ColorRGBA{}, graphics.ColorFromRGBA(101, 112, 130, 128)},
		{"scrollbar-color:transparent transparent", graphics.ColorRGBA{}, graphics.ColorRGBA{}},
		{"scrollbar-color:auto", defaults.Track.Background.Fill, defaults.Thumb.Normal.Background.Fill},
		{transparentTrack, graphics.ColorRGBA{}, graphics.ColorFromRGBA(101, 112, 130, 128)},
	} {
		if node, ok := htmlui.NodeByID(world, owner, "notes"); ok {
			node.SetAttribute("style", baseStyle+test.css)
		}
		for range 2 {
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
		}
		textarea := htmlControlByID(t, world, owner, "notes")
		override, ok := ecs.Get[StyleOverride[ScrollStyle]](world, textarea)
		if !ok || override.Value.Track.Background.Fill != test.track || override.Value.Thumb.Normal.Background.Fill != test.thumb {
			t.Fatalf("%s: scrollbar style = %+v, ok=%v", test.css, override.Value, ok)
		}
		if test.css == transparentTrack && (override.Value.Thumb.Hovered == nil || override.Value.Thumb.Hovered.Background.Fill.A != 0) {
			t.Fatalf("explicit transparent hover color was ignored: %+v", override.Value.Thumb.Hovered)
		}
		seenTrack, seenThumb := false, false
		for entity, access := range ecs.Query2[Relation[SurfaceStyle], scene.Size2D](world).All {
			if access.A.Get().Owner != textarea || access.B.Get().Height <= 0 {
				continue
			}
			background, _ := ecs.Get[scene.UIBackground2D](world, entity)
			if ecs.Has[Part[ScrollTrackVerticalPart]](world, entity) {
				seenTrack = true
				if background.Shape.Fill != test.track {
					t.Fatalf("%s: painted track = %v, want %v", test.css, background.Shape.Fill, test.track)
				}
			}
			if ecs.Has[Part[ScrollThumbVerticalPart]](world, entity) {
				seenThumb = true
				if background.Shape.Fill != test.thumb {
					t.Fatalf("%s: painted thumb = %v, want %v", test.css, background.Shape.Fill, test.thumb)
				}
			}
		}
		if !seenTrack || !seenThumb {
			t.Fatalf("%s: scroll geometry missing, track=%v thumb=%v", test.css, seenTrack, seenThumb)
		}
	}
}

func TestHTMLButtonTextKeepsHTMLLayoutPositions(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<button id="create" style="width:100px">创建并打开</button>`)
	html.Width, html.Height = 160, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	button := htmlControlByID(t, world, owner, "create")
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	minX, maxX := float32(1<<30), float32(-1<<30)
	count := 0
	for _, access := range ecs.Query3[htmlui.ControlText, scene.Text2D, scene.UITransform](world).All {
		link := access.A.Mut()
		access.B.Mut()
		transform := access.C.Mut()

		if link.Control != button {
			continue
		}
		x := transform.Translation.X.Value
		minX, maxX = min(minX, x), max(maxX, x)
		count++
	}
	if count < 2 || maxX <= minX {
		t.Fatalf("HTML button text positions = count:%d range:%v..%v; labels must not overlap", count, minX, maxX)
	}
}

func TestEmptyHTMLInputCreatesValueVisual(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<input id="empty" placeholder="输入你的名字">`)
	html.Width, html.Height = 240, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "empty")
	count := 0
	for entity, access := range ecs.Query2[htmlui.ControlText, scene.Text2D](world).All {
		link := access.A.Get()
		text := access.B.Get()

		if link.Control != field {
			continue
		}
		count++
		if text.Value != "输入你的名字" || !ecs.Has[Part[ValuePart]](world, entity) {
			t.Fatalf("HTML input value visual = value:%q entity:%d", text.Value, entity)
		}
	}
	if count != 1 {
		t.Fatalf("empty HTML input value visuals = %d, want 1", count)
	}
}

func TestHTMLButtonSynchronizesHoverStyles(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<button id="window-close">Close</button><style>#window-close{background:#112233}#window-close:hover{background:#445566}</style>`)
	html.Width, html.Height = 240, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	button := htmlControlByID(t, world, owner, "window-close")
	buttonSurface := func() graphics.ColorRGBA {
		surface, ok := ecs.Get[scene.Sprite2D](world, button)
		if !ok {
			t.Fatal("HTML button surface was not generated")
		}
		return surface.Shape.Fill
	}
	if got := buttonSurface(); got != graphics.ColorFromRGBA(17, 34, 51, 255) {
		t.Fatalf("normal HTML button color = %+v", got)
	}

	ecs.WriteMessage(world, scene.InputEvent{Entity: button, Kind: scene.InputEventEnter})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	button = htmlControlByID(t, world, owner, "window-close")
	buttonSurface = func() graphics.ColorRGBA {
		surface, ok := ecs.Get[scene.Sprite2D](world, button)
		if !ok {
			t.Fatal("hovered HTML button surface was not generated")
		}
		return surface.Shape.Fill
	}
	if got := buttonSurface(); got != graphics.ColorFromRGBA(68, 85, 102, 255) {
		t.Fatalf("hovered HTML button color = %+v", got)
	}
}

func TestHTMLInputSynchronizesFocusStyles(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<input id="first"><input id="second"><style>input{border:1px solid #112233}input:focus{border-color:#445566}</style>`)
	html.Width, html.Height = 320, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	first := htmlControlByID(t, world, owner, "first")
	second := htmlControlByID(t, world, owner, "second")
	borderColor := func(entity ecs.Entity) graphics.ColorRGBA {
		surface, ok := ecs.Get[scene.Sprite2D](world, entity)
		if !ok {
			t.Fatalf("HTML input surface was not generated for %d", entity)
		}
		return surface.Shape.Border
	}
	if got := borderColor(first); got != graphics.ColorFromRGBA(17, 34, 51, 255) {
		t.Fatalf("unfocused HTML input border = %+v", got)
	}

	ecs.WriteMessage(world, scene.InputEvent{Entity: first, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := borderColor(first); got != graphics.ColorFromRGBA(68, 85, 102, 255) {
		t.Fatalf("focused HTML input border = %+v", got)
	}

	ecs.WriteMessage(world, scene.InputEvent{Entity: second, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := borderColor(first); got != graphics.ColorFromRGBA(17, 34, 51, 255) {
		t.Fatalf("blurred HTML input border = %+v", got)
	}
	if got := borderColor(second); got != graphics.ColorFromRGBA(68, 85, 102, 255) {
		t.Fatalf("second focused HTML input border = %+v", got)
	}
}

func TestHTMLInputTextKeepsLayoutPositionAcrossRebuild(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div><input id="search" value="搜索资源"><input id="rename" value="世界观.md"></div><style>div{display:flex;gap:10px}input{width:128px;height:30px;padding:5px 7px;font-size:12px}</style>`)
	html.Width, html.Height = 320, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	for range 2 {
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}

	before := htmlInputTextTransforms(t, world, owner)
	for _, id := range []string{"search", "rename"} {
		node, ok := htmlui.NodeByID(world, owner, id)
		if !ok || !node.SetAttribute("data-rebuild", "1") {
			t.Fatalf("mark HTML input %q dirty", id)
		}
	}
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	afterRebuild := htmlInputTextTransforms(t, world, owner)
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	afterStable := htmlInputTextTransforms(t, world, owner)
	for _, id := range []string{"search", "rename"} {
		want := before[id]
		if got := afterRebuild[id]; got != want {
			t.Errorf("%s text moved during HTML rebuild: before=%+v after=%+v", id, want, got)
		}
		if got := afterStable[id]; got != want {
			t.Errorf("%s text moved during post-rebuild widget sync: before=%+v after=%+v", id, want, got)
		}
	}
}

func TestHTMLInputTextScrollDoesNotAccumulate(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<input id="field" value="第一章.md"><style>*{box-sizing:border-box}input{width:90px;height:32px;padding:5px 7px;font:17px ui}</style>`)
	html.Width, html.Height = 120, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	for range 2 {
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	field := htmlControlByID(t, world, owner, "field")
	state, _ := ecs.Get[TextField](world, field)
	if state.ScrollX >= 0 {
		t.Fatalf("HTML input scroll = %v, want negative offset at end caret", state.ScrollX)
	}
	textX := func() float32 {
		var x float32
		for _, access := range ecs.Query3[htmlui.ControlText, scene.Text2D, scene.UITransform](world).All {
			link := access.A.Mut()
			access.B.Mut()
			transform := access.C.Mut()

			if link.Control == field {
				x = transform.Translation.X.Value
				break
			}
		}
		return x
	}
	before := textX()
	for range 5 {
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	if after := textX(); absFloat32(after-before) > 0.001 {
		t.Fatalf("HTML input text origin drifted during stable sync: before=%v after=%v", before, after)
	}
}

func TestHTMLInputInsertionKeepsTextBeforeCaretAnchored(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<input id="field" value="12345678901234567890"><style>*{box-sizing:border-box}input{width:90px;height:32px;padding:5px 7px;font:17px ui}</style>`)
	html.Width, html.Height = 120, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	for range 2 {
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	field := htmlControlByID(t, world, owner, "field")
	fieldState := ecs.GetMut[TextField](world, field)
	fieldState.Focused, fieldState.Caret, fieldState.Anchor = true, 10, 10
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	var text *scene.Text2D
	var transform *scene.UITransform
	for _, access := range ecs.Query3[htmlui.ControlText, scene.Text2D, scene.UITransform](world).All {
		link := access.A.Mut()
		value := access.B.Mut()
		position := access.C.Mut()

		if link.Control == field {
			text, transform = value, position
			break
		}
	}
	if text == nil || transform == nil {
		t.Fatal("HTML input text visual is missing")
	}
	leftBefore, _, _, _, _ := scene.TextCaretRect2D(text, text.Value, 9)
	rightBefore, _, _, _, _ := scene.TextCaretRect2D(text, text.Value, 11)
	leftBefore += transform.Translation.X.Value
	rightBefore += transform.Translation.X.Value
	fieldState.Value, fieldState.Caret, fieldState.Anchor = "1234567890X1234567890", 11, 11
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	leftAfter, _, _, _, _ := scene.TextCaretRect2D(text, fieldState.Value, 9)
	rightAfter, _, _, _, _ := scene.TextCaretRect2D(text, fieldState.Value, 12)
	leftAfter += transform.Translation.X.Value
	rightAfter += transform.Translation.X.Value
	if diff := absFloat32(leftAfter - leftBefore); diff > 0.001 {
		t.Fatalf("text before insertion caret moved: before=%v after=%v", leftBefore, leftAfter)
	}
	if rightAfter <= rightBefore {
		t.Fatalf("text after insertion caret did not move right: before=%v after=%v", rightBefore, rightAfter)
	}
}

func TestHTMLSelectTextIsVerticallyCenteredWithTightPadding(t *testing.T) {
	for _, display := range []string{"display:inline-block", "position:absolute;top:10px;left:10px"} {
		for _, height := range []string{"28px", "48px"} {
			t.Run(display+"/"+height, func(t *testing.T) {
				world := ecs.NewWorld()
				RegisterRuntimeFeature[HTMLRenderer](world)
				surface := htmlui.New(`<select id="mode"><option>none</option><option>System 系统</option></select><style>select{box-sizing:border-box;width:180px;height:` + height + `;padding:7px 9px;border:1px solid #3b404b;font:14px sans-serif;` + display + `}</style>`)
				surface.Width, surface.Height, surface.Font = 240, 120, notosanssc.NotoSansSCRegular
				bundle := ecs.NewBundle(1)
				ecs.Put(&bundle, surface)
				owner := world.Spawn(bundle)
				if err := SyncSystem("").Run(world); err != nil {
					t.Fatal(err)
				}
				control := htmlControlByID(t, world, owner, "mode")
				for _, selected := range []int{0, 1, 0} {
					SetHTMLControlSelected(world, control, selected)
					if err := SyncSystem("").Run(world); err != nil {
						t.Fatal(err)
					}
					size := ecs.GetRef[scene.Size2D](world, control)
					found := false
					for _, access := range ecs.Query3[htmlui.ControlText, scene.Text2D, scene.UITransform](world).All {
						if access.A.Ref().Control != control {
							continue
						}
						found = true
						text, transform := access.B.Ref(), access.C.Ref()
						_, lineHeight, ok := scene.MeasureTextValue2D(text, text.Value)
						if !ok {
							t.Fatal("select label could not be measured")
						}
						if midpoint := transform.Translation.Y.Value + lineHeight*0.5; absFloat32(midpoint-size.Height*0.5) > 0.001 {
							t.Fatalf("select label midpoint=%v, control midpoint=%v (text y=%v, line height=%v)", midpoint, size.Height*0.5, transform.Translation.Y.Value, lineHeight)
						}
						if transform.Translation.X.Value != 10 {
							t.Fatalf("select lost horizontal padding: x=%v, want 10", transform.Translation.X.Value)
						}
					}
					if !found {
						t.Fatal("select label is missing")
					}
				}
			})
		}
	}
}

func TestHTMLInputTextAndCaretAreVerticallyCentered(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	html := htmlui.New(`<input id="field" value="value"><style>*{box-sizing:border-box}input{width:180px;height:38px;padding:7px 10px;font:14px ui}</style>`)
	html.Width, html.Height = 240, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	var controlHeight float32
	if controlSize, ok := ecs.Get[scene.Size2D](world, field); ok {
		controlHeight = controlSize.Height
	} else {
		t.Fatal("HTML input control has no size")
	}

	var textY, lineHeight float32
	foundText := false
	for _, access := range ecs.Query3[htmlui.ControlText, scene.Text2D, scene.UITransform](world).All {
		link := access.A.Mut()
		text := access.B.Mut()
		transform := access.C.Mut()

		if link.Control != field {
			continue
		}
		_, lineHeight, _ = scene.MeasureTextValue2D(text, text.Value)
		textY = transform.Translation.Y.Value
		foundText = true
		break
	}
	if !foundText {
		t.Fatal("HTML input value text was not generated")
	}
	wantY := (controlHeight - lineHeight) * 0.5
	if diff := absFloat32(textY - wantY); diff > 0.001 {
		t.Fatalf("HTML input text y=%v, want %v", textY, wantY)
	}

	if fieldState := ecs.GetMut[TextField](world, field); fieldState != nil {
		fieldState.Focused = true
	} else {
		t.Fatal("HTML input has no text field state")
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	var caretY, caretHeight float32
	foundCaret := false
	for _, access := range ecs.Query3[htmlui.ControlVisual, scene.UITransform, scene.Size2D](world).All {
		visual := access.A.Mut()
		transform := access.B.Mut()
		size := access.C.Mut()

		if visual.Control != field || visual.Selection {
			continue
		}
		caretY, caretHeight = transform.Translation.Y.Value, size.Height
		foundCaret = true
		break
	}
	if !foundCaret {
		t.Fatal("HTML input caret was not generated")
	}
	if diff := absFloat32(caretY - wantY); diff > 0.001 {
		t.Fatalf("HTML input caret y=%v, want %v", caretY, wantY)
	}
	if diff := absFloat32(caretHeight - lineHeight); diff > 0.001 {
		t.Fatalf("HTML input caret height=%v, want %v", caretHeight, lineHeight)
	}
}

func absFloat32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}

func htmlInputTextTransforms(t *testing.T, world *ecs.World, owner ecs.Entity) map[string]scene.UITransform {
	t.Helper()
	transforms := make(map[string]scene.UITransform, 2)
	controls := make(map[ecs.Entity]string, 2)
	for entity, access := range ecs.Query[htmlui.Control](world).All {
		control := access.Get()

		if control.Owner == owner && (control.ID == "search" || control.ID == "rename") {
			controls[entity] = control.ID
		}
	}
	for _, access := range ecs.Query2[htmlui.ControlText, scene.UITransform](world).All {
		link := access.A.Get()
		transform := access.B.Get()

		if id, ok := controls[link.Control]; ok {
			transforms[id] = transform
		}
	}
	if len(transforms) != len(controls) {
		t.Fatalf("HTML input text transforms = %d, want %d", len(transforms), len(controls))
	}
	return transforms
}

func TestHTMLControlRuntimeDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<input id="name" value="Onix">`)
	html.Width, html.Height = 240, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	world.Spawn(bundle)
	syncSystem, updateSystem := SyncSystem(""), UpdateSystem("")
	for range 2 {
		if err := updateSystem.Run(world); err != nil {
			t.Fatal(err)
		}
		if err := syncSystem.Run(world); err != nil {
			t.Fatal(err)
		}
	}

	if allocations := testing.AllocsPerRun(100, func() {
		if err := updateSystem.Run(world); err != nil {
			panic(err)
		}
		if err := syncSystem.Run(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("stable HTML control runtime allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLCheckboxTogglesAndUpdatesCheckedState(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<input id="enabled" type="checkbox" checked>`)
	html.Width, html.Height = 220, 40
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	checkbox := htmlControlByID(t, world, owner, "enabled")
	control, _ := ecs.Get[htmlui.Control](world, checkbox)
	if !control.Checked || !ecs.Has[Toggle[bool]](world, checkbox) {
		t.Fatalf("checkbox state = %#v", control)
	}
	checkedMark := false
	controlSize, _ := ecs.Get[scene.Size2D](world, checkbox)
	for _, access := range ecs.Query3[scene.Parent, scene.Sprite2D, scene.UITransform](world).All {
		parent := access.A.Mut()
		mark := access.B.Mut()
		transform := access.C.Mut()

		if parent != nil && mark != nil && transform != nil && parent.Entity == checkbox && mark.Shape.Kind == render.ShapeCheckmark {
			want := (controlSize.Width - mark.Width) * 0.5
			if absFloat32(transform.Translation.X.Value-want) > 0.001 || absFloat32(transform.Translation.Y.Value-want) > 0.001 {
				t.Fatalf("checkbox mark position = (%v, %v), want (%v, %v)", transform.Translation.X.Value, transform.Translation.Y.Value, want, want)
			}
			checkedMark = true
			break
		}
	}
	if !checkedMark {
		t.Fatal("checked checkbox did not render its mark")
	}
	reader := &ecs.MessageReader[htmlui.Event]{}
	ecs.WriteMessage(world, scene.InputEvent{Entity: checkbox, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ = ecs.Get[htmlui.Control](world, checkbox)
	if control.Checked {
		t.Fatalf("checkbox remained checked: %#v", control)
	}
	markHidden := false
	for _, access := range ecs.Query3[scene.Parent, scene.Sprite2D, scene.Visibility](world).All {
		parent := access.A.Mut()
		mark := access.B.Mut()
		visibility := access.C.Mut()

		if parent != nil && mark != nil && visibility != nil && parent.Entity == checkbox && mark.Shape.Kind == render.ShapeCheckmark {
			markHidden = !visibility.Visible
			break
		}
	}
	if !markHidden {
		t.Fatal("unchecked checkbox mark remained visible")
	}
	if events := reader.Read(world); len(events) != 3 || events[2].Kind != htmlui.EventChange || events[2].Checked {
		t.Fatalf("checkbox events = %#v", events)
	}
}

func TestHTMLCheckboxWithoutIDToggles(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<input type="checkbox">`)
	html.Width, html.Height = 80, 30
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	var checkbox ecs.Entity
	for entity, access := range ecs.Query[htmlui.Control](world).All {
		control := access.Get()

		if control.Owner == owner && control.Tag == "input" && control.Type == "checkbox" {
			checkbox = entity
			break
		}
	}
	if checkbox == 0 {
		t.Fatal("checkbox without id was not generated")
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: checkbox, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ := ecs.Get[htmlui.Control](world, checkbox)
	if !control.Checked {
		t.Fatalf("checkbox without id did not toggle: %#v", control)
	}
}

func TestHTMLCheckboxUpdatesCheckedCSSWithoutRebuild(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<input id="enabled" type="checkbox" checked><style>input{background:#112233}input:checked{background:#445566}</style>`)
	html.Width, html.Height = 80, 30
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	checkbox := htmlControlByID(t, world, owner, "enabled")
	surface, _ := ecs.Get[scene.Sprite2D](world, checkbox)
	if surface.Shape.Fill != graphics.ColorFromRGB(68, 85, 102) {
		t.Fatalf("checked CSS fill = %#v", surface.Shape.Fill)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: checkbox, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	surface, _ = ecs.Get[scene.Sprite2D](world, checkbox)
	if surface.Shape.Fill != graphics.ColorFromRGB(17, 34, 51) {
		t.Fatalf("unchecked CSS fill = %#v", surface.Shape.Fill)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	markVisible := true
	for _, access := range ecs.Query3[scene.Parent, scene.Sprite2D, scene.Visibility](world).All {
		parent := access.A.Mut()
		mark := access.B.Mut()
		visibility := access.C.Mut()

		if parent != nil && mark != nil && visibility != nil && parent.Entity == checkbox && mark.Shape.Kind == render.ShapeCheckmark {
			markVisible = visibility.Visible
			break
		}
	}
	if markVisible {
		t.Fatal("unchecked checkbox mark became visible after HTML sync")
	}
}

func TestHTMLCheckboxKeepsAuthoredCheckedAttribute(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<input id="enabled" type="checkbox" checked>`)
	html.Width, html.Height = 80, 30
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	checkbox := htmlControlByID(t, world, owner, "enabled")
	ecs.WriteMessage(world, scene.InputEvent{Entity: checkbox, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ := ecs.Get[htmlui.Control](world, checkbox)
	if control.Checked || !control.DefaultChecked {
		t.Fatalf("checkbox live/default checkedness = %v/%v", control.Checked, control.DefaultChecked)
	}
	node, ok := htmlui.NodeByID(world, owner, "enabled")
	if !ok || !node.SetAttribute("checked", "") {
		t.Fatal("checked content attribute was not retained")
	}
}

func TestHTMLActiveStylesFollowPointerLifecycle(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<button id="press">Press</button><style>#press{background:#112233}#press:active{background:#445566}</style>`)
	html.Width, html.Height = 120, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	button := htmlControlByID(t, world, owner, "press")
	fill := func() graphics.ColorRGBA {
		sprite, _ := ecs.Get[scene.Sprite2D](world, button)
		return sprite.Shape.Fill
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: button, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := fill(); got != graphics.ColorFromRGB(0x44, 0x55, 0x66) {
		t.Fatalf("active button fill = %#v", got)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: button, Kind: scene.InputEventRelease})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := fill(); got != graphics.ColorFromRGB(0x11, 0x22, 0x33) {
		t.Fatalf("released button fill = %#v", got)
	}
}

func TestHTMLControlInteractionStylesKeepEntityStable(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<input id="sibling" value="keep"><button id="action">Action</button><style>#action{background:transparent}#action:hover{background:#334455}#action:active{background:#445566}#action:focus-visible{background:#556677}</style>`)
	html.Width, html.Height = 120, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	button := htmlControlByID(t, world, owner, "action")
	sibling := htmlControlByID(t, world, owner, "sibling")
	assertStable := func(want graphics.ColorRGBA) {
		t.Helper()
		if !world.IsAlive(button) || !world.IsAlive(sibling) {
			t.Fatal("interaction rebuilt the HTML tree and destroyed an existing control")
		}
		if current := htmlControlByID(t, world, owner, "action"); current != button {
			t.Fatalf("interaction rebuilt HTML button: before=%d after=%d", button, current)
		}
		if current := htmlControlByID(t, world, owner, "sibling"); current != sibling {
			t.Fatalf("interaction rebuilt HTML sibling: before=%d after=%d", sibling, current)
		}
		surface, ok := ecs.Get[scene.Sprite2D](world, button)
		if !ok {
			t.Fatal("HTML button surface was removed")
		}
		if surface.Shape.Fill != want {
			t.Fatalf("HTML button fill = %#v, want %#v", surface.Shape.Fill, want)
		}
	}
	runInput := func(kind scene.InputEventKind, want graphics.ColorRGBA) {
		t.Helper()
		ecs.WriteMessage(world, scene.InputEvent{Entity: button, Kind: kind})
		if err := UpdateSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		assertStable(want)
	}
	runFocus := func(focused bool, want graphics.ColorRGBA) {
		t.Helper()
		ecs.WriteMessage(world, FocusChangedEvent{Entity: button, Focused: focused, FocusVisible: focused})
		if err := UpdateSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		assertStable(want)
	}

	assertStable(graphics.ColorRGBA{})
	runInput(scene.InputEventEnter, graphics.ColorFromRGB(0x33, 0x44, 0x55))
	runInput(scene.InputEventPress, graphics.ColorFromRGB(0x44, 0x55, 0x66))
	runInput(scene.InputEventCancel, graphics.ColorFromRGB(0x33, 0x44, 0x55))
	runInput(scene.InputEventPress, graphics.ColorFromRGB(0x44, 0x55, 0x66))
	runInput(scene.InputEventTap, graphics.ColorFromRGB(0x44, 0x55, 0x66))
	runInput(scene.InputEventRelease, graphics.ColorFromRGB(0x33, 0x44, 0x55))
	runInput(scene.InputEventLeave, graphics.ColorRGBA{})
	runFocus(true, graphics.ColorFromRGB(0x55, 0x66, 0x77))
	runFocus(false, graphics.ColorRGBA{})
	if allocations := testing.AllocsPerRun(100, func() {
		if !htmlui.SetControlHover(world, button, true) || !htmlui.SetControlHover(world, button, false) {
			panic("failed to update HTML control hover")
		}
	}); allocations > 4 {
		// Interning is per cascade pass, so each hovered and unhovered state
		// stores one instance instead of mutating the shared one.
		t.Fatalf("HTML control hover allocated %.0f objects, want at most 4", allocations)
	}
	assertStable(graphics.ColorRGBA{})
}

func TestHTMLFocusWithinUpdatesAncestorSurface(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<div id="wrap"><input id="field"></div><style>#wrap{background:#112233;padding:2px}#wrap:focus-within{background:#445566}</style>`)
	html.Width, html.Height = 220, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	ecs.WriteMessage(world, scene.InputEvent{Entity: field, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, access := range ecs.Query2[scene.Sprite2D, scene.Parent](world).All {
		sprite := access.A.Get()
		parent := access.B.Get()

		if parent.Entity == owner && sprite.Shape.Fill == graphics.ColorFromRGB(0x44, 0x55, 0x66) {
			found = true
		}
	}
	if !found {
		t.Fatal("focus-within did not repaint the ancestor surface")
	}
}

func TestHTMLInputValueUpdatesPlaceholderShownStyles(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<input id="field" placeholder=""><style>#field{background:#112233}#field:placeholder-shown{background:#445566}</style>`)
	html.Width, html.Height = 180, 40
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	if !htmlui.SetControlValue(world, field, "typed") {
		t.Fatal("live value update failed")
	}
	sprite, _ := ecs.Get[scene.Sprite2D](world, field)
	if sprite.Shape.Fill != graphics.ColorFromRGB(0x11, 0x22, 0x33) {
		t.Fatalf("filled input retained placeholder-shown style: %#v", sprite.Shape.Fill)
	}
}

func TestHTMLControlValueSetterSynchronizesTextField(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<textarea id="editor">old</textarea>`)
	html.Width, html.Height = 180, 80
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	editor := htmlControlByID(t, world, owner, "editor")
	content := "package main\n"
	if !SetHTMLControlValue(world, editor, content) {
		t.Fatal("textarea live value update failed")
	}
	control, _ := ecs.Get[htmlui.Control](world, editor)
	field, _ := ecs.Get[TextField](world, editor)
	if control.Value != content || field.Value != content || field.Caret != 3 || field.Anchor != 3 {
		t.Fatalf("textarea value was not synchronized: control=%+v field=%+v", control, field)
	}
}

func TestHTMLControlValueFilledAfterEmptyRenderKeepsCSSColor(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<textarea id="editor" style="color:transparent"></textarea>`)
	html.Width, html.Height, html.Font = 180, 80, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	editor := htmlControlByID(t, world, owner, "editor")
	readEditorText := func() (scene.Text2D, bool) {
		var text scene.Text2D
		found := false
		for _, access := range ecs.Query2[htmlui.ControlText, scene.Text2D](world).All {
			link := access.A.Get()
			value := access.B.Get()

			if link.Control == editor {
				text, found = value, true
				break
			}
		}
		return text, found
	}
	text, found := readEditorText()
	if !found || text.Color.A != 0 {
		t.Fatalf("empty editor text color = %#v, found=%v; want transparent CSS color", text.Color, found)
	}
	content := "package main\n"
	if !SetHTMLControlValue(world, editor, content) {
		t.Fatal("textarea live value update failed")
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	text, found = readEditorText()
	if !found || text.Value != content || text.Color.A != 0 {
		t.Fatalf("filled editor text = %#v, found=%v; want source with transparent CSS color", text, found)
	}
}

func TestHTMLOrdinaryElementHoverUpdatesDescendantStyle(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<div id="card" style="background:#111111;padding:2px"><span>Label</span></div><style>#card:hover span{color:#445566}</style>`)
	html.Width, html.Height = 180, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	var card ecs.Entity
	for entity, access := range ecs.Query2[scene.Sprite2D, scene.Parent](world).All {
		sprite := access.A.Get()
		parent := access.B.Get()

		if parent.Entity == owner && sprite.Shape.Fill == graphics.ColorFromRGB(0x11, 0x11, 0x11) {
			card = entity
			break
		}
	}
	if card == 0 {
		t.Fatal("ordinary HTML surface did not expose an input target")
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: card, Kind: scene.InputEventEnter})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, access := range ecs.Query[scene.Text2D](world).All {
		text := access.Get()

		if text.Value == "Label" && text.Color == graphics.ColorFromRGB(0x44, 0x55, 0x66) {
			found = true
		}
	}
	if !found {
		t.Fatal("ordinary element hover did not repaint descendant text")
	}
}

func TestHTMLHoverCreatesPreviouslyTransparentSurface(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<div id="card" style="width:120px;height:32px">Label</div><style>#card:hover{background:#445566}</style>`)
	html.Width, html.Height = 180, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	var target ecs.Entity
	for entity, access := range ecs.Query[scene.Text2D](world).All {
		text := access.Get()

		if text.Value == "Label" && ecs.Has[scene.InputTarget2D](world, entity) {
			target = entity
			break
		}
	}
	if target == 0 {
		t.Fatal("transparent element had no descendant hit target")
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: target, Kind: scene.InputEventEnter})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, access := range ecs.Query2[scene.Sprite2D, scene.Parent](world).All {
		sprite := access.A.Get()
		parent := access.B.Get()

		if parent.Entity == owner && sprite.Shape.Fill == graphics.ColorFromRGB(0x44, 0x55, 0x66) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("hover did not rebuild a surface for the newly painted background")
	}
}

func TestHTMLHoverRebuildsMixedBorderSurfaces(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<div id="card" style="width:120px;height:32px">Label</div><style>#card{border:2px solid #112233}#card:hover{border-left-color:#445566}</style>`)
	html.Width, html.Height = 180, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	var target ecs.Entity
	for entity, access := range ecs.Query2[scene.Sprite2D, scene.Parent](world).All {
		sprite := access.A.Get()
		parent := access.B.Get()

		if parent.Entity == owner && sprite.Shape.Border == graphics.ColorFromRGB(0x11, 0x22, 0x33) {
			target = entity
			break
		}
	}
	if target == 0 {
		t.Fatal("uniform border surface was not generated")
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: target, Kind: scene.InputEventEnter})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, access := range ecs.Query2[scene.Sprite2D, scene.Parent](world).All {
		sprite := access.A.Get()
		parent := access.B.Get()

		if parent.Entity == owner && sprite.Shape.Fill == graphics.ColorFromRGB(0x44, 0x55, 0x66) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("mixed hover border did not rebuild its left-edge surface")
	}
}

func TestHTMLDisabledControlStillMatchesHover(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<button id="button" disabled>Button</button><style>#button{background:#112233}#button:disabled:hover{background:#445566}</style>`)
	html.Width, html.Height = 160, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	button := htmlControlByID(t, world, owner, "button")
	target, _ := ecs.Get[scene.InputTarget2D](world, button)
	if !target.Disabled || !target.AllowDisabledInput {
		t.Fatalf("disabled HTML target policy = %#v", target)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: button, Kind: scene.InputEventEnter})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	sprite, _ := ecs.Get[scene.Sprite2D](world, button)
	if sprite.Shape.Fill != graphics.ColorFromRGB(0x44, 0x55, 0x66) {
		t.Fatalf("disabled hovered button fill = %#v", sprite.Shape.Fill)
	}
}

func TestHTMLTabOrderAndFocusVisible(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	html := htmlui.New(`<button id="second" tabindex="2">Second</button><button id="first" tabindex="1">First</button><button id="skipped" tabindex="-1">Skipped</button><button id="normal">Normal</button><style>button{background:#112233}button:focus-visible{background:#445566}</style>`)
	html.Width, html.Height = 420, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	first := htmlControlByID(t, world, owner, "first")
	second := htmlControlByID(t, world, owner, "second")
	normal := htmlControlByID(t, world, owner, "normal")
	pressTab := func() {
		uiInputState.PressKey(input.KeyTab)
		if err := UpdateSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		uiInputState.ReleaseKey(input.KeyTab)
		uiInputState.EndFrame()
	}
	for _, want := range []ecs.Entity{first, second, normal} {
		pressTab()
		focus, _ := ecs.GetResource[*scene.InputFocus](world)
		if focus == nil || focus.Keyboard != want {
			t.Fatalf("tab focus = %v, want %v", focus, want)
		}
		sprite, _ := ecs.Get[scene.Sprite2D](world, want)
		if sprite.Shape.Fill != graphics.ColorFromRGB(0x44, 0x55, 0x66) {
			t.Fatalf("keyboard-focused control fill = %#v", sprite.Shape.Fill)
		}
	}
}

func TestHTMLTabOrderIncludesOrdinaryElementsAndEditingHosts(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	html := htmlui.New(`<div id="second" tabindex="2">Second</div><button id="normal">Normal</button><div id="first" tabindex="1">First</div><a id="link" href="#next">Link</a><div id="editable" contenteditable>Edit</div><div id="negative" tabindex="-1">Negative</div><div id="invalid" tabindex="nope">Invalid</div><a id="nohref">No link</a><div id="outer" contenteditable>Outer<span id="nested" contenteditable>Nested</span></div><style>div:focus-visible{background:#445566}</style>`)
	html.Width, html.Height = 480, 220
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	first := htmlFocusableByID(t, world, owner, "first")
	second := htmlFocusableByID(t, world, owner, "second")
	normal := htmlFocusableByID(t, world, owner, "normal")
	link := htmlFocusableByID(t, world, owner, "link")
	editable := htmlFocusableByID(t, world, owner, "editable")
	outer := htmlFocusableByID(t, world, owner, "outer")
	negative := htmlFocusableByID(t, world, owner, "negative")
	for _, id := range []string{"invalid", "nohref", "nested"} {
		if entity := findHTMLFocusable(world, owner, id); entity != 0 {
			t.Fatalf("%s generated unexpected focus target %d", id, entity)
		}
	}
	pressTab := func() ecs.Entity {
		uiInputState.PressKey(input.KeyTab)
		if err := UpdateSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		uiInputState.ReleaseKey(input.KeyTab)
		uiInputState.EndFrame()
		focus, _ := ecs.GetResource[*scene.InputFocus](world)
		return focus.Keyboard
	}
	for _, want := range []ecs.Entity{first, second, normal, link, editable, outer} {
		if got := pressTab(); got != want {
			t.Fatalf("tab focus = %d, want %d", got, want)
		}
	}
	if got := pressTab(); got != first {
		t.Fatalf("wrapped tab focus = %d, want %d", got, first)
	}

	ecs.WriteMessage(world, scene.InputEvent{Entity: negative, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	focus, _ := ecs.GetResource[*scene.InputFocus](world)
	if focus.Keyboard != negative {
		t.Fatalf("pointer focus for tabindex=-1 = %d, want %d", focus.Keyboard, negative)
	}
}

func TestHTMLFocusTraversalDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<div tabindex="2">Second</div><button>Normal</button><span tabindex="1">First</span><div contenteditable>Editor</div>`)
	html.Width, html.Height = 320, 100
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.GetResource[*uiEventState](world)
	for range 4 {
		focusNextHTMLElement(world, state, false)
		ecs.AdvanceMessages(world)
	}
	allocations := testing.AllocsPerRun(100, func() {
		focusNextHTMLElement(world, state, false)
		ecs.AdvanceMessages(world)
	})
	if allocations != 0 {
		t.Fatalf("HTML focus traversal allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLFocusOwnerPreservesDescendantHoverTarget(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<div id="editor" tabindex="0"><span id="child">Child</span></div><style>#child{color:#112233}#child:hover{color:#445566}</style>`)
	html.Width, html.Height = 180, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	focusOwner := htmlFocusableByID(t, world, owner, "editor")
	var childText ecs.Entity
	for entity, access := range ecs.Query[scene.Text2D](world).All {
		text := access.Get()

		if text.Value == "Child" {
			childText = entity
			break
		}
	}
	ownerLink, ok := ecs.Get[htmlui.FocusOwner](world, childText)
	if childText == 0 || !ok || ownerLink.Entity != focusOwner {
		t.Fatalf("descendant focus owner = text:%d link:%#v", childText, ownerLink)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: childText, Kind: scene.InputEventEnter})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	text, _ := ecs.Get[scene.Text2D](world, childText)
	if text.Color != graphics.ColorFromRGB(0x44, 0x55, 0x66) {
		t.Fatalf("descendant hover color = %#v", text.Color)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: childText, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	focus, _ := ecs.GetResource[*scene.InputFocus](world)
	if focus.Keyboard != focusOwner {
		t.Fatalf("descendant pointer focus = %d, want %d", focus.Keyboard, focusOwner)
	}
}

func TestHTMLContentEditablePointerFocusMatchesFocusVisible(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<div id="editor" contenteditable style="width:120px;height:32px">Edit</div><style>#editor{background:#112233}#editor:focus-visible{background:#445566}</style>`)
	html.Width, html.Height = 180, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	target := htmlFocusableByID(t, world, owner, "editor")
	var surface ecs.Entity
	for entity, access := range ecs.Query2[scene.Sprite2D, scene.Parent](world).All {
		sprite := access.A.Get()
		parent := access.B.Get()

		if parent.Entity == owner && sprite.Shape.Fill == graphics.ColorFromRGB(0x11, 0x22, 0x33) {
			surface = entity
			break
		}
	}
	route, routeOK := ecs.Get[scene.InputTarget2D](world, surface)
	focusOwner, focusOwnerOK := ecs.Get[htmlui.FocusOwner](world, surface)
	if surface == 0 || !routeOK || route.Receiver != 0 || !focusOwnerOK || focusOwner.Entity != target {
		t.Fatalf("editing host input route = surface:%d target:%d route:%#v focus-owner:%#v", surface, target, route, focusOwner)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: surface, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, access := range ecs.Query2[scene.Sprite2D, scene.Parent](world).All {
		sprite := access.A.Get()
		parent := access.B.Get()

		if parent.Entity == owner && sprite.Shape.Fill == graphics.ColorFromRGB(0x44, 0x55, 0x66) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("pointer-focused editing host did not match :focus-visible")
	}
}

func TestHTMLOrdinaryFocusSurvivesLayoutUpdate(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	html := htmlui.New(`<div id="panel" tabindex="0">Panel</div><style>#panel:focus{width:180px}</style>`)
	html.Width, html.Height = 240, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	previous := htmlFocusableByID(t, world, owner, "panel")
	uiInputState.PressKey(input.KeyTab)
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	uiInputState.ReleaseKey(input.KeyTab)
	uiInputState.EndFrame()
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	current := htmlFocusableByID(t, world, owner, "panel")
	if current != previous || !world.IsAlive(previous) {
		t.Fatalf("focus layout change replaced target: old=%d new=%d", previous, current)
	}
	focus, _ := ecs.GetResource[*scene.InputFocus](world)
	if focus.Keyboard != current {
		t.Fatalf("rebuilt ordinary focus = %d, want %d", focus.Keyboard, current)
	}
}

func TestHTMLResetButtonSynchronizesRuntimeControls(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<form id="settings"><input id="field" value="default"><input id="check" type="checkbox" checked><select id="mode"><option>A</option><option selected>B</option></select><button id="reset" type="reset">Reset</button></form>`)
	html.Width, html.Height = 420, 100
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	check := htmlControlByID(t, world, owner, "check")
	mode := htmlControlByID(t, world, owner, "mode")
	reset := htmlControlByID(t, world, owner, "reset")
	if !htmlui.SetControlValue(world, field, "edited") || !htmlui.SetControlChecked(world, check, false) || !htmlui.SetControlSelected(world, mode, 0) {
		t.Fatal("failed to dirty form controls")
	}
	if text := ecs.GetMut[TextField](world, field); text != nil {
		text.Value = "edited"
	}
	if toggle := ecs.GetMut[Toggle[bool]](world, check); toggle != nil {
		toggle.Value = false
	}
	if selector := ecs.GetMut[Selector](world, mode); selector != nil {
		selector.Selected = 0
	}
	reader := &ecs.MessageReader[htmlui.Event]{}
	ecs.WriteMessage(world, scene.InputEvent{Entity: reset, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	fieldState, _ := ecs.Get[htmlui.Control](world, field)
	checkState, _ := ecs.Get[htmlui.Control](world, check)
	modeState, _ := ecs.Get[htmlui.Control](world, mode)
	text, _ := ecs.Get[TextField](world, field)
	toggle, _ := ecs.Get[Toggle[bool]](world, check)
	selector, _ := ecs.Get[Selector](world, mode)
	if fieldState.Value != "default" || text.Value != "default" || !checkState.Checked || !toggle.Value || modeState.Selected != 1 || selector.Selected != 1 {
		t.Fatalf("reset runtime state = field:%#v text:%#v check:%#v toggle:%#v mode:%#v selector:%#v", fieldState, text, checkState, toggle, modeState, selector)
	}
	assertHTMLEvent(t, reader.Read(world), owner, "settings", htmlui.EventReset, "")
}

func TestHTMLResetDefaultActionCanBePreventedBeforeStateChanges(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<form id="settings"><input id="field" value="default"><button id="reset" type="reset">Reset</button></form>`)
	html.Width, html.Height = 300, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	reset := htmlControlByID(t, world, owner, "reset")
	if !htmlui.SetControlValue(world, field, "dirty") {
		t.Fatal("dirty field")
	}
	seenPreReset := false
	if err := ecs.Insert(world, owner, htmlui.ActionHandler{Handle: func(context *htmlui.ActionContext) {
		if context.Event.Kind != htmlui.EventReset {
			return
		}
		control, _ := ecs.Get[htmlui.Control](world, field)
		seenPreReset = control.Value == "dirty"
		context.PreventDefault()
	}}); err != nil {
		t.Fatal(err)
	}
	reader := &ecs.MessageReader[htmlui.Event]{}
	ecs.WriteMessage(world, scene.InputEvent{Entity: reset, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ := ecs.Get[htmlui.Control](world, field)
	if !seenPreReset || control.Value != "dirty" || !control.DirtyValue {
		t.Fatalf("cancelled reset changed state: pre=%v control=%#v", seenPreReset, control)
	}
	found := false
	for _, event := range reader.Read(world) {
		if event.Kind == htmlui.EventReset {
			found = event.Bubbles && event.Cancelable && event.DefaultPrevented
		}
	}
	if !found {
		t.Fatal("cancelled reset event did not retain Chromium event flags")
	}
}

func TestHTMLResetButtonWithoutFormOnlyEmitsClick(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<button id="reset" type="reset">Reset</button>`)
	html.Width, html.Height = 160, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	reset := htmlControlByID(t, world, owner, "reset")
	reader := &ecs.MessageReader[htmlui.Event]{}
	ecs.WriteMessage(world, scene.InputEvent{Entity: reset, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	events := reader.Read(world)
	if len(events) != 1 || events[0].Kind != htmlui.EventClick {
		t.Fatalf("formless reset events = %#v", events)
	}
}

func TestHTMLMultipleSelectKeyboardAnchorAndRange(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	html := htmlui.New(`<select id="mode" multiple size="5"><option>A</option><option>B</option><option disabled>C</option><option>D</option><option>E</option></select>`)
	html.Width, html.Height = 180, 160
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	mode := htmlControlByID(t, world, owner, "mode")
	ecs.WriteMessage(world, scene.InputEvent{Entity: mode, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	press := func(key input.KeyCode, shift, shortcut bool) {
		if shift {
			uiInputState.PressKey(input.KeyLeftShift)
		}
		if shortcut {
			uiInputState.PressKey(input.KeyLeftControl)
		}
		uiInputState.PressKey(key)
		ecs.WriteMessage(world, input.Event{Seq: uint64(key) + 1, Kind: input.EventKeyDown, Key: key})
		if err := FocusedInputSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		uiInputState.ReleaseKey(key)
		uiInputState.ReleaseKey(input.KeyLeftShift)
		uiInputState.ReleaseKey(input.KeyLeftControl)
		uiInputState.EndFrame()
	}
	press(input.KeyDown, false, false)
	press(input.KeyDown, true, false)
	press(input.KeyDown, true, false)
	control, _ := ecs.Get[htmlui.Control](world, mode)
	if control.SelectionAnchor != 0 || control.SelectionFocus != 3 || control.SelectedCount != 3 ||
		!control.Options[0].Selected || !control.Options[1].Selected || control.Options[2].Selected || !control.Options[3].Selected {
		t.Fatalf("shift range = %#v", control)
	}
	for entity, access := range ecs.Query[SelectorItem](world).All {
		item := access.Get()

		if item.Owner != mode {
			continue
		}
		state, _ := ecs.Get[Control](world, entity)
		want := item.Index == 0 || item.Index == 1 || item.Index == 3
		if state.Selected != want {
			t.Fatalf("option %d visual selected = %v, want %v", item.Index, state.Selected, want)
		}
	}
	press(input.KeyUp, false, true)
	control, _ = ecs.Get[htmlui.Control](world, mode)
	if control.SelectionFocus != 1 || control.SelectedCount != 3 {
		t.Fatalf("ctrl navigation changed selection = %#v", control)
	}
	press(input.KeySpace, false, true)
	control, _ = ecs.Get[htmlui.Control](world, mode)
	if control.Options[1].Selected || control.SelectedCount != 2 {
		t.Fatalf("ctrl-space did not toggle navigation focus = %#v", control)
	}
	selectNode, _ := htmlui.NodeByID(world, owner, "mode")
	if !selectNode.SetAttribute("data-rebuild", "1") {
		t.Fatal("mark multiple select surface dirty")
	}
	previous := mode
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	mode = htmlControlByID(t, world, owner, "mode")
	control, _ = ecs.Get[htmlui.Control](world, mode)
	focus, _ := ecs.GetResource[*scene.InputFocus](world)
	if mode != previous || focus.Keyboard != mode || control.SelectionAnchor != 0 || control.SelectionFocus != 1 || control.SelectedCount != 2 {
		t.Fatalf("multiple selection rebuild state = old:%d new:%d focus:%#v control:%#v", previous, mode, focus, control)
	}
}

func TestHTMLMultipleSelectKeyboardDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	html := htmlui.New(`<select id="mode" multiple size="3"><option>A</option><option>B</option><option>C</option></select>`)
	html.Width, html.Height = 180, 100
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	mode := htmlControlByID(t, world, owner, "mode")
	press := func(key input.KeyCode) {
		uiInputState.PressKey(key)
		context := scene.FocusedInputContext{World: world, Entity: mode, Event: input.Event{Kind: input.EventKeyDown, Key: key}}
		handleHTMLSelectInput(&context)
		if !context.Handled {
			panic("multiple select key was not consumed")
		}
		uiInputState.ReleaseKey(key)
		uiInputState.EndFrame()
		ecs.AdvanceMessages(world)
	}
	for range 4 {
		press(input.KeyDown)
		press(input.KeyUp)
	}
	allocations := testing.AllocsPerRun(100, func() {
		press(input.KeyDown)
		press(input.KeyUp)
	})
	if allocations != 0 {
		t.Fatalf("multiple-select keyboard update allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLContentEditableLiveTextSelectionAndCancelableBeforeInput(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	html := htmlui.New(`<div id="editor" contenteditable style="width:160px;height:48px">ab</div>`)
	html.Width, html.Height = 220, 80
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	editor := htmlFocusableByID(t, world, owner, "editor")
	editable, ok := ecs.Get[htmlui.Editable](world, editor)
	field, fieldOK := ecs.Get[TextField](world, editor)
	if !ok || !fieldOK || editable.Value != "ab" || field.Value != "ab" {
		t.Fatalf("contenteditable runtime = editable:%#v field:%#v", editable, field)
	}
	textCount, caretCount, selectionCount := 0, 0, 0
	for _, access := range ecs.Query[htmlui.EditableText](world).All {
		text := access.Get()

		if text.Editable == editor {
			textCount++
		}
	}
	for _, access := range ecs.Query[htmlui.EditableVisual](world).All {
		visual := access.Get()

		if visual.Editable == editor {
			if visual.Selection {
				selectionCount++
			} else {
				caretCount++
			}
		}
	}
	if textCount != 1 || caretCount != 1 || selectionCount != 16 {
		t.Fatalf("contenteditable visuals = text:%d caret:%d selection:%d", textCount, caretCount, selectionCount)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: editor, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if !SetHTMLEditableSelection(world, editor, 1, 1) {
		t.Fatal("set contenteditable Selection")
	}
	reader := &ecs.MessageReader[htmlui.Event]{}
	uiInputState.CommitText("X")
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	editable, _ = ecs.Get[htmlui.Editable](world, editor)
	if editable.Value != "aXb" || editable.Anchor != 2 || editable.Focus != 2 {
		t.Fatalf("edited contenteditable = %#v", editable)
	}
	events := reader.Read(world)
	if len(events) < 2 || events[len(events)-2].Kind != htmlui.EventBeforeInput || events[len(events)-2].Value != "ab" ||
		!events[len(events)-2].Cancelable || events[len(events)-1].Kind != htmlui.EventInput || events[len(events)-1].Value != "aXb" || events[len(events)-1].InputType != "insertText" {
		t.Fatalf("contenteditable events = %#v", events)
	}
	uiInputState.EndFrame()
	cancelled := false
	if err := ecs.Insert(world, owner, htmlui.ActionHandler{Handle: func(context *htmlui.ActionContext) {
		if context.Event.Kind == htmlui.EventBeforeInput {
			cancelled = true
			context.PreventDefault()
		}
	}}); err != nil {
		t.Fatal(err)
	}
	uiInputState.CommitText("Y")
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field, _ = ecs.Get[TextField](world, editor)
	if !cancelled || field.Value != "aXb" {
		t.Fatalf("prevented beforeinput changed value: cancelled=%v field=%#v", cancelled, field)
	}
	uiInputState.EndFrame()
	previous := editor
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	editor = htmlFocusableByID(t, world, owner, "editor")
	focus, _ := ecs.GetResource[*scene.InputFocus](world)
	field, _ = ecs.Get[TextField](world, editor)
	if editor != previous || focus.Keyboard != editor || field.Value != "aXb" || field.Caret != 2 || field.Anchor != 2 {
		t.Fatalf("contenteditable rebuild state = old:%d new:%d focus:%#v field:%#v", previous, editor, focus, field)
	}
}

func TestHTMLContentEditableStablePathsDoNotAllocate(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	ecs.InsertResource(world, uiInputState)
	html := htmlui.New(`<div id="editor" contenteditable style="width:160px;height:48px">text</div>`)
	html.Width, html.Height = 220, 80
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	editor := htmlFocusableByID(t, world, owner, "editor")
	ecs.WriteMessage(world, scene.InputEvent{Entity: editor, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if !htmlui.SetEditableSelection(world, editor, 0, 2) {
		t.Fatal("warm contenteditable Selection")
	}
	selectionAllocations := testing.AllocsPerRun(100, func() {
		if !htmlui.SetEditableSelection(world, editor, 0, 2) {
			panic("contenteditable Selection failed")
		}
	})
	if selectionAllocations != 0 {
		t.Fatalf("contenteditable Selection allocated %.0f objects, want 0", selectionAllocations)
	}
	uiInputState.EndFrame()
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	idleAllocations := testing.AllocsPerRun(100, func() {
		if err := UpdateSystem("").Run(world); err != nil {
			panic(err)
		}
	})
	if idleAllocations != 0 {
		t.Fatalf("idle contenteditable update allocated %.0f objects, want 0", idleAllocations)
	}
}

func TestHTMLMultipleSelectItemUsesChromiumModifierSelection(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<select id="mode" multiple><option selected>A</option><option>B</option><option selected>C</option></select>`)
	html.Width, html.Height = 180, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	mode := htmlControlByID(t, world, owner, "mode")
	var item ecs.Entity
	for entity, access := range ecs.Query[SelectorItem](world).All {
		candidate := access.Get()

		if candidate.Owner == mode && candidate.Index == 1 {
			item = entity
			break
		}
	}
	if item == 0 {
		t.Fatal("multiple select item was not generated")
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: item, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ := ecs.Get[htmlui.Control](world, mode)
	selector, _ := ecs.Get[Selector](world, mode)
	if control.SelectedCount != 1 || control.Options[0].Selected || !control.Options[1].Selected || control.Options[2].Selected || !selector.Open || control.SelectionAnchor != 1 || control.SelectionFocus != 1 {
		t.Fatalf("multiple plain-click state = control:%#v selector:%#v", control, selector)
	}
	uiInputState, _ := ecs.GetResource[*input.State](world)
	uiInputState.PressKey(input.KeyLeftControl)
	ecs.WriteMessage(world, scene.InputEvent{Entity: item, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ = ecs.Get[htmlui.Control](world, mode)
	if control.SelectedCount != 0 || control.Options[1].Selected {
		t.Fatalf("multiple ctrl-click did not toggle target = %#v", control)
	}
}

func TestHTMLAutofocusCreatesRealFocusOnce(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<button id="auto" autofocus>Auto</button><button id="other">Other</button><style>#auto{background:#112233}#auto:focus{background:#445566}</style>`)
	html.Width, html.Height = 240, 60
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	auto := htmlControlByID(t, world, owner, "auto")
	focus, _ := ecs.GetResource[*scene.InputFocus](world)
	if focus == nil || focus.Keyboard != auto {
		t.Fatalf("autofocus target = %v, want %v", focus, auto)
	}
	sprite, _ := ecs.Get[scene.Sprite2D](world, auto)
	if sprite.Shape.Fill != graphics.ColorFromRGB(0x44, 0x55, 0x66) {
		t.Fatalf("autofocused button fill = %#v", sprite.Shape.Fill)
	}
	input, _ := ecs.GetResource[*uiEventState](world)
	focusWithVisibility(world, input, 0, false)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if input.Focused != 0 {
		t.Fatalf("autofocus reapplied after blur: %v", input.Focused)
	}
}

func TestHTMLRadioActivationUpdatesOneLiveCheckedState(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<form><input id="first" type="radio" name="mode" checked><input id="second" type="radio" name="mode"></form><style>input{background:#112233}input:checked{background:#445566}</style>`)
	html.Width, html.Height = 100, 40
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	first := htmlControlByID(t, world, owner, "first")
	second := htmlControlByID(t, world, owner, "second")
	ecs.WriteMessage(world, scene.InputEvent{Entity: second, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	firstState, _ := ecs.Get[htmlui.Control](world, first)
	secondState, _ := ecs.Get[htmlui.Control](world, second)
	if firstState.Checked || !secondState.Checked || !firstState.DefaultChecked || secondState.DefaultChecked {
		t.Fatalf("radio live/default state = first:%#v second:%#v", firstState, secondState)
	}
	firstSprite, _ := ecs.Get[scene.Sprite2D](world, first)
	secondSprite, _ := ecs.Get[scene.Sprite2D](world, second)
	if firstSprite.Shape.Fill != graphics.ColorFromRGB(0x11, 0x22, 0x33) || secondSprite.Shape.Fill != graphics.ColorFromRGB(0x44, 0x55, 0x66) {
		t.Fatalf("radio checked styles = first:%#v second:%#v", firstSprite.Shape.Fill, secondSprite.Shape.Fill)
	}
}

func TestHTMLFocusedButtonSurvivesStateLayoutUpdate(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<button id="button">Button</button><style>#button{box-sizing:border-box;width:100px}#button:focus{width:140px}</style>`)
	html.Width, html.Height = 200, 50
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	before := htmlControlByID(t, world, owner, "button")
	ecs.WriteMessage(world, scene.InputEvent{Entity: before, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	after := htmlControlByID(t, world, owner, "button")
	if before != after || !world.IsAlive(before) {
		t.Fatalf("layout state replaced control: before=%v after=%v", before, after)
	}
	focus, _ := ecs.GetResource[*scene.InputFocus](world)
	input, _ := ecs.GetResource[*uiEventState](world)
	if focus == nil || focus.Keyboard != after || input == nil || input.Focused != after {
		t.Fatalf("focus after layout rebuild = input:%v input:%v want:%v", focus, input, after)
	}
	size, _ := ecs.Get[scene.Size2D](world, after)
	if size.Width != 140 {
		t.Fatalf("focused rebuilt width = %v, want 140", size.Width)
	}
}

func TestHTMLSelectKeepsOneCurrentLabelVisualAcrossSyncs(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<select id="look"><option>未选择妆造</option><option selected>默认妆造</option><option>战斗妆造</option></select>`)
	html.Width, html.Height = 180, 40
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selectEntity := htmlControlByID(t, world, owner, "look")
	count := 0
	value := ""
	for _, access := range ecs.Query2[htmlui.ControlText, scene.Text2D](world).All {
		link := access.A.Get()
		text := access.B.Get()

		if link.Control == selectEntity {
			count++
			value = text.Value
		}
	}
	if count != 1 || value != "默认妆造" {
		t.Fatalf("select label visuals = %d with value %q, want one %q", count, value, "默认妆造")
	}
}

func TestHTMLSelectOpensAndSelectsOption(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<select id="mode"><option value="a">Alpha</option><option value="b" selected>Beta</option></select>`)
	html.Width, html.Height = 180, 40
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selectEntity := htmlControlByID(t, world, owner, "mode")
	control, _ := ecs.Get[htmlui.Control](world, selectEntity)
	if control.Selected != 1 || control.Value != "b" || len(control.Options) != 2 || !ecs.Has[Selector](world, selectEntity) {
		t.Fatalf("select state = %#v", control)
	}
	selectLabel := func() string {
		var value string
		for _, access := range ecs.Query2[htmlui.ControlText, scene.Text2D](world).All {
			link := access.A.Get()
			text := access.B.Get()

			if link.Control == selectEntity {
				value = text.Value
				break
			}
		}
		return value
	}
	if got := selectLabel(); got != "Beta" {
		t.Fatalf("initial select label = %q, want Beta", got)
	}
	indicatorFound := false
	for _, access := range ecs.Query3[scene.Parent, scene.UIImage2D, scene.Size2D](world).All {
		parent := access.A.Mut()
		image := access.B.Mut()
		size := access.C.Mut()

		if parent != nil && image != nil && size != nil && parent.Entity == selectEntity && image.Texture != nil && size.Width > 0 && size.Height > 0 {
			indicatorFound = true
			break
		}
	}
	if !indicatorFound {
		t.Fatal("select dropdown icon was not rendered")
	}
	textIndicatorFound := false
	for _, access := range ecs.Query2[scene.Parent, scene.Text2D](world).All {
		parent := access.A.Get()
		text := access.B.Get()

		if parent.Entity == selectEntity && text.Value == "v" {
			textIndicatorFound = true
			break
		}
	}
	if textIndicatorFound {
		t.Fatal("select dropdown indicator still uses text v")
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: selectEntity, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selector, _ := ecs.Get[Selector](world, selectEntity)
	if !selector.Open {
		t.Fatal("select did not open")
	}
	listVisible := false
	itemVisible := false
	popupEscapesControlClip := false
	for entity, access := range ecs.Query2[SelectorList, scene.Visibility](world).All {
		list := access.A.Mut()
		visibility := access.B.Mut()

		if list != nil && visibility != nil && list.Selector == selectEntity {
			listVisible = visibility.Visible
			parent, parentOK := ecs.Get[scene.Parent](world, entity)
			popupEscapesControlClip = parentOK && parent.Entity == selectEntity && ecs.Has[scene.ClipExempt2D](world, entity)
		}
	}
	for _, access := range ecs.Query2[SelectorItem, scene.Visibility](world).All {
		item := access.A.Mut()
		visibility := access.B.Mut()

		if item != nil && visibility != nil && item.Owner == selectEntity {
			itemVisible = itemVisible || visibility.Visible
		}
	}
	if !listVisible || !itemVisible {
		t.Fatalf("select popup visibility = list %v, item %v", listVisible, itemVisible)
	}
	if !popupEscapesControlClip {
		t.Fatal("select popup remained inside the clipped control hierarchy")
	}
	popupTextFont := false
	for _, access := range ecs.Query2[scene.Parent, scene.Text2D](world).All {
		parent := access.A.Mut()
		text := access.B.Mut()

		if parent == nil || text == nil {
			continue
		}
		item, itemOK := ecs.Get[SelectorItem](world, parent.Entity)
		if itemOK && item.Owner == selectEntity && text.Font == html.Font {
			popupTextFont = true
			break
		}
	}
	if !popupTextFont {
		t.Fatal("select popup option did not inherit the HTML font")
	}
	popupTextVisible := false
	popupTextAbovePanel := false
	for entity, access := range ecs.Query2[scene.Parent, scene.Text2D](world).All {
		parent := access.A.Get()
		text := access.B.Get()

		item, itemOK := ecs.Get[SelectorItem](world, parent.Entity)
		if !itemOK || item.Owner != selectEntity || text.Value != "Alpha" {
			continue
		}
		visibility, visibleOK := ecs.Get[scene.Visibility](world, entity)
		stack, stackOK := ecs.Get[scene.UIStackIndex](world, entity)
		popupTextVisible = visibleOK && visibility.Visible
		popupTextAbovePanel = stackOK && int(stack) > selectorPopupStackBase+1
		break
	}
	if !popupTextVisible || !popupTextAbovePanel {
		t.Fatalf("select popup option text visibility/stack = visible:%v abovePanel:%v", popupTextVisible, popupTextAbovePanel)
	}
	var option ecs.Entity
	for entity, access := range ecs.Query[SelectorItem](world).All {
		item := access.Get()

		if item.Owner == selectEntity && item.Index == 0 {
			option = entity
			break
		}
	}
	if option == 0 {
		t.Fatal("select option was not generated")
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: option, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ = ecs.Get[htmlui.Control](world, selectEntity)
	if control.Selected != 0 || control.Value != "a" || control.Open {
		t.Fatalf("selected option state = %#v", control)
	}
	if got := selectLabel(); got != "Alpha" {
		t.Fatalf("selected option label = %q, want Alpha", got)
	}
}

func TestHTMLSelectPopupKeepsItsOwnBoundsAndRoundedClip(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	parent := SpawnPanel(world, PanelOptions{Width: 240, Height: 180})
	html := htmlui.New(`<div id="panel"><select id="mode" style="width:180px;height:28px;border-radius:3px"><option>Alpha</option><option selected>Beta</option></select></div>`)
	html.Width, html.Height, html.Font = 240, 180, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := ecs.Insert(world, owner, scene.Parent{Entity: parent}); err != nil {
		t.Fatal(err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selectEntity := htmlControlByID(t, world, owner, "mode")
	for _, step := range []struct {
		name         string
		open, hidden bool
		visible      bool
	}{
		{name: "initially closed"},
		{name: "opened", open: true, visible: true},
		{name: "closed after opening"},
		{name: "reopened", open: true, visible: true},
		{name: "hidden ancestor", open: true, hidden: true},
		{name: "shown again", open: true, visible: true},
	} {
		t.Run(step.name, func(t *testing.T) {
			selector := ecs.GetMut[Selector](world, selectEntity)
			selector.Open = step.open
			parentVisibility := ecs.GetMut[scene.Visibility](world, parent)
			parentVisibility.Visible = !step.hidden
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if err := SyncAfterLayoutSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			runtime := ecs.GetRef[htmlSelectorRuntime](world, selectEntity)
			controlSize := ecs.GetRef[scene.Size2D](world, selectEntity)
			transform := ecs.GetRef[scene.UITransform](world, runtime.List)
			if gap := transform.Translation.Y.Value - controlSize.Height; gap != 4 {
				t.Fatalf("popup gap=%v, want 4", gap)
			}
			size := ecs.GetRef[scene.Size2D](world, runtime.List)
			clip := ecs.GetRef[scene.RoundedClip2D](world, runtime.List)
			if clip == nil || clip.Width != size.Width || clip.Height != size.Height || clip.Radius != 3 {
				t.Fatalf("popup does not clip all option backgrounds to its rounded bounds: clip=%+v, size=%+v", clip, size)
			}
			for _, entity := range runtime.Elements {
				visibility := ecs.GetRef[scene.Visibility](world, entity)
				if visibility == nil || visibility.Visible != step.visible {
					t.Errorf("popup element %d visibility = %+v, want %v", entity, visibility, step.visible)
				}
			}
		})
	}
	// Fitting the picker above the control must retain the same rounded clip,
	// including when the viewport is too short to show all options.
	ecs.SetIfChanged(world, parent, scene.UITransformFromXY(0, 30))
	if err := ecs.Insert(world, owner, scene.NewUITransform()); err != nil {
		t.Fatal(err)
	}
	if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	ecs.InsertResource(world, scene.ViewportSize{Width: 240, Height: 65})
	ecs.GetMut[Selector](world, selectEntity).Upward = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	runtime := ecs.GetRef[htmlSelectorRuntime](world, selectEntity)
	size := ecs.GetRef[scene.Size2D](world, runtime.List)
	transform := ecs.GetRef[scene.UITransform](world, runtime.List)
	clip := ecs.GetRef[scene.RoundedClip2D](world, runtime.List)
	if size.Height >= 60 || clip.Height != size.Height || transform.Translation.Y.Value != -size.Height-4 {
		t.Fatalf("picker did not fit above the control: size=%+v, clip=%+v, y=%v", size, clip, transform.Translation.Y.Value)
	}
}

func TestHTMLSelectClosesWhenFocusLeaves(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<select id="mode"><option>Alpha</option><option selected>Beta</option></select><button id="outside">Outside</button>`)
	html.Width, html.Height = 240, 80
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selectEntity := htmlControlByID(t, world, owner, "mode")
	outside := htmlControlByID(t, world, owner, "outside")
	ecs.WriteMessage(world, scene.InputEvent{Entity: selectEntity, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selector, _ := ecs.Get[Selector](world, selectEntity)
	if !selector.Open {
		t.Fatal("select did not open")
	}

	ecs.WriteMessage(world, scene.InputEvent{Entity: outside, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ := ecs.Get[htmlui.Control](world, selectEntity)
	selector, _ = ecs.Get[Selector](world, selectEntity)
	if selector.Open || control.Open {
		t.Fatalf("select remained open after focus left: selector=%v control=%v", selector.Open, control.Open)
	}
	listVisible := false
	for _, access := range ecs.Query2[SelectorList, scene.Visibility](world).All {
		list := access.A.Mut()
		visibility := access.B.Mut()

		if list != nil && list.Selector == selectEntity && visibility != nil {
			listVisible = visibility.Visible
		}
	}
	if listVisible {
		t.Fatal("select popup remained visible after focus left")
	}

	// Focus loss must close the popup even when it is delivered directly by
	// the focus manager instead of a pointer press on another control.
	ecs.WriteMessage(world, scene.InputEvent{Entity: selectEntity, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	ecs.WriteMessage(world, FocusChangedEvent{Entity: selectEntity, Focused: false})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selector, _ = ecs.Get[Selector](world, selectEntity)
	control, _ = ecs.Get[htmlui.Control](world, selectEntity)
	if selector.Open || control.Open {
		t.Fatalf("select remained open after direct focus loss: selector=%v control=%v", selector.Open, control.Open)
	}
}

func TestHTMLSelectPopupUsesControlSurfaceRadiusAndBorder(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<select id="mode" style="background:#112233;border-radius:0"><option>Alpha</option><option selected>Beta</option></select>`)
	html.Width, html.Height = 180, 40
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selectEntity := htmlControlByID(t, world, owner, "mode")
	control, _ := ecs.Get[scene.Sprite2D](world, selectEntity)
	controlSize, _ := ecs.Get[scene.Size2D](world, selectEntity)
	var panel ecs.Entity
	var border ecs.Entity
	var listY float32
	for entity, _ := range ecs.Query[Part[SelectorPanelPart]](world).All {
		parent, parentOK := ecs.Get[scene.Parent](world, entity)
		if !parentOK {
			continue
		}
		list, listOK := ecs.Get[SelectorList](world, parent.Entity)
		if listOK && list.Selector == selectEntity {
			if transform, transformOK := ecs.Get[scene.UITransform](world, parent.Entity); transformOK {
				listY = transform.Translation.Y.Value
			}
			panel = entity
			break
		}
	}
	if panel == 0 {
		t.Fatal("select popup panel was not generated")
	}
	list, _ := ecs.Get[scene.Parent](world, panel)
	for entity, _ := range ecs.Query[Part[SelectorPanelBorderPart]](world).All {
		parent, parentOK := ecs.Get[scene.Parent](world, entity)
		if parentOK && parent.Entity == list.Entity {
			border = entity
			break
		}
	}
	if border == 0 {
		t.Fatalf("select popup border = %d, want generated", border)
	}
	background, _ := ecs.Get[scene.UIBackground2D](world, panel)
	radius, _ := ecs.Get[CornerRadius](world, panel)
	borderSurface, _ := ecs.Get[scene.UIBorder2D](world, border)
	if background.Shape.Fill != control.Shape.Fill {
		t.Fatalf("select popup fill = %#v, want control fill %#v", background.Shape.Fill, control.Shape.Fill)
	}
	if borderSurface.Shape.BorderWidth != 1 {
		t.Fatalf("select popup border width = %v, want 1", borderSurface.Shape.BorderWidth)
	}
	if absFloat32(listY-controlSize.Height-4) > 0.001 {
		t.Fatalf("select popup y = %v, want control height %v plus 4px gap", listY, controlSize.Height)
	}
	if background.Shape.Radius != 0 || radius != CornerRadius(0) {
		t.Fatalf("select popup radius = shape:%v component:%v, want 0 from CSS", background.Shape.Radius, radius)
	}
}

func TestHTMLDisabledSelectOptionDoesNotChange(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<select id="mode"><option disabled>Unavailable</option><option selected>Available</option></select>`)
	html.Width, html.Height = 180, 40
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selectEntity := htmlControlByID(t, world, owner, "mode")
	option := ecs.Entity(0)
	for entity, access := range ecs.Query[SelectorItem](world).All {
		item := access.Get()

		if item.Owner == selectEntity && item.Index == 0 {
			option = entity
			break
		}
	}
	if option == 0 {
		t.Fatal("disabled select option was not generated")
	}
	reader := &ecs.MessageReader[htmlui.Event]{}
	ecs.WriteMessage(world, scene.InputEvent{Entity: option, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control, _ := ecs.Get[htmlui.Control](world, selectEntity)
	if control.Selected != 1 || control.Value != "Available" {
		t.Fatalf("disabled option changed select = %#v", control)
	}
	if events := reader.Read(world); len(events) != 0 {
		t.Fatalf("disabled option emitted events = %#v", events)
	}
}

func TestHTMLChoiceStateSurvivesRebuildAndSelectorPopupCleansUp(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())
	html := htmlui.New(`<div id="root"><input id="enabled" type="checkbox" checked><select id="mode"><option>A</option><option selected>B</option></select></div>`)
	html.Width, html.Height = 240, 80
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	checkbox := htmlControlByID(t, world, owner, "enabled")
	selectEntity := htmlControlByID(t, world, owner, "mode")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: checkbox, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selector, _ := ecs.Get[Selector](world, selectEntity)
	selector.Open = true
	if current := ecs.GetMut[Selector](world, selectEntity); current != nil {
		*current = selector
	}
	if current := ecs.GetMut[htmlui.Control](world, selectEntity); current != nil {
		current.Open = true
	}
	root, _ := htmlui.NodeByID(world, owner, "root")
	root.SetAttribute("data-rebuild", "1")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	checkbox = htmlControlByID(t, world, owner, "enabled")
	control, _ := ecs.Get[htmlui.Control](world, checkbox)
	if control.Checked {
		t.Fatal("checkbox checked state was lost during HTML rebuild")
	}
	selectEntity = htmlControlByID(t, world, owner, "mode")
	control, _ = ecs.Get[htmlui.Control](world, selectEntity)
	if control.Selected != 1 {
		t.Fatalf("select selected state was lost during HTML rebuild: %#v", control)
	}
	selector, ok := ecs.Get[Selector](world, selectEntity)
	if !ok || !selector.Open {
		t.Fatalf("select open state was lost during HTML rebuild: %#v, ok=%v", selector, ok)
	}
	listVisible := false
	itemVisible := false
	for _, access := range ecs.Query2[SelectorList, scene.Visibility](world).All {
		list := access.A.Mut()
		visibility := access.B.Mut()

		if list != nil && visibility != nil && list.Selector == selectEntity {
			listVisible = visibility.Visible
		}
	}
	for _, access := range ecs.Query2[SelectorItem, scene.Visibility](world).All {
		item := access.A.Mut()
		visibility := access.B.Mut()

		if item != nil && visibility != nil && item.Owner == selectEntity {
			itemVisible = itemVisible || visibility.Visible
		}
	}
	if !listVisible || !itemVisible {
		t.Fatalf("select popup visibility was lost during HTML rebuild: list=%v items=%v", listVisible, itemVisible)
	}
	if err := htmlui.Set(world, owner, `<div>replacement</div>`); !err {
		t.Fatal("replace HTML source")
	}
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	remaining := 0
	for range ecs.Query[htmlSelectorElement](world).All {
		remaining++
	}
	if remaining != 0 {
		t.Fatalf("stale selector popup elements = %d", remaining)
	}
}

func htmlControlByID(t *testing.T, world *ecs.World, owner ecs.Entity, id string) ecs.Entity {
	t.Helper()
	var found ecs.Entity
	for entity, access := range ecs.Query[htmlui.Control](world).All {
		control := access.Get()

		if control.Owner == owner && control.ID == id {
			found = entity
		}
		if !(found == 0) {
			break
		}
	}
	if found == 0 {
		t.Fatalf("HTML control %q was not generated", id)
	}
	return found
}

func findHTMLFocusable(world *ecs.World, owner ecs.Entity, id string) ecs.Entity {
	var found ecs.Entity
	for entity, access := range ecs.Query[htmlui.Focusable](world).All {
		focusable := access.Get()

		if focusable.Owner == owner && focusable.ID == id {
			found = entity
			break
		}
	}
	return found
}

func htmlFocusableByID(t *testing.T, world *ecs.World, owner ecs.Entity, id string) ecs.Entity {
	t.Helper()
	entity := findHTMLFocusable(world, owner, id)
	if entity == 0 {
		t.Fatalf("HTML focusable %q was not generated", id)
	}
	return entity
}

func assertHTMLEvent(t *testing.T, events []htmlui.Event, owner ecs.Entity, id string, kind htmlui.EventKind, value string) {
	t.Helper()
	for _, event := range events {
		if event.Entity == owner && event.ID == id && event.Kind == kind && event.Value == value {
			return
		}
	}
	t.Fatalf("missing HTML event owner=%d id=%q kind=%q value=%q in %#v", owner, id, kind, value, events)
}

func TestHTMLTextareaEnterAndExplicitBlur(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	state := input.NewState()
	ecs.InsertResource(world, state)
	html := htmlui.New(`<textarea id="notes">hello</textarea>`)
	html.Width, html.Height, html.Font = 320, 160, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "notes")
	ecs.WriteMessage(world, scene.InputEvent{Entity: entity, Kind: scene.InputEventPress})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := ecs.GetMut[TextField](world, entity)
	field.Caret, field.Anchor = 5, 5
	state.EndFrame()
	state.PressKey(input.KeyEnter)
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field = ecs.GetMut[TextField](world, entity)
	if field.Value != "hello\n" || !field.Focused || field.Submitted {
		t.Fatalf("textarea Enter: %+v", field)
	}
	BlurHTMLControl(world, entity)
	field = ecs.GetMut[TextField](world, entity)
	runtime, _ := ecs.GetResource[*uiEventState](world)
	if field.Focused || runtime.Focused != 0 {
		t.Fatal("explicit HTML blur retained focus")
	}
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	var reader ecs.MessageReader[htmlui.Event]
	assertHTMLEvent(t, reader.Read(world), owner, "notes", htmlui.EventBlur, "hello\n")
}

func TestHTMLButtonSpaceActivatesOnReleaseAndCancelsOnBlur(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	state := input.NewState()
	ecs.InsertResource(world, state)
	html := htmlui.New(`<button id="action" type="button">Action</button>`)
	html.Width, html.Height, html.Font = 160, 60, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "action")
	runtime, _ := ecs.GetResource[*uiEventState](world)
	if runtime == nil {
		if err := UpdateSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		runtime, _ = ecs.GetResource[*uiEventState](world)
	}
	var reader ecs.MessageReader[htmlui.Event]
	clicks := func() int {
		count := 0
		for _, event := range reader.Read(world) {
			if event.Kind == htmlui.EventClick {
				count++
			}
		}
		return count
	}
	for _, blur := range []bool{false, true} {
		focus(world, runtime, entity)
		state.EndFrame()
		state.PressKey(input.KeySpace)
		if err := UpdateSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if clicks() != 0 {
			t.Fatal("Space activated before release")
		}
		if blur {
			BlurHTMLControl(world, entity)
			focus(world, runtime, entity)
		}
		state.EndFrame()
		state.ReleaseKey(input.KeySpace)
		if err := UpdateSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		want := 1
		if blur {
			want = 0
		}
		if got := clicks(); got != want {
			t.Fatalf("Space clicks after blur=%v: %d, want %d", blur, got, want)
		}
	}
}

func TestHTMLRetainedControlsRefreshAuthoredAttributes(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<input id="field" placeholder="Before"/><input id="check" type="checkbox"/><div id="editor" contenteditable="true">Before</div><select id="select"><option id="option">Before</option></select>`)
	html.Width, html.Height, html.Font = 320, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncHTML(world); err != nil {
		t.Fatal(err)
	}
	fieldEntity := htmlControlByID(t, world, owner, "field")
	checkEntity := htmlControlByID(t, world, owner, "check")
	selectEntity := htmlControlByID(t, world, owner, "select")
	editorEntity := htmlFocusableByID(t, world, owner, "editor")
	fieldNode, _ := htmlui.NodeByID(world, owner, "field")
	fieldNode.SetAttribute("placeholder", "After")
	fieldNode.SetAttribute("maxlength", "4")
	fieldNode.SetAttribute("value", "New")
	checkNode, _ := htmlui.NodeByID(world, owner, "check")
	checkNode.SetAttribute("checked", "")
	editorNode, _ := htmlui.NodeByID(world, owner, "editor")
	editorNode.SetText("After")
	optionNode, _ := htmlui.NodeByID(world, owner, "option")
	optionNode.SetText("After")
	if err := SyncHTML(world); err != nil {
		t.Fatal(err)
	}
	if htmlControlByID(t, world, owner, "field") != fieldEntity || htmlControlByID(t, world, owner, "check") != checkEntity || htmlControlByID(t, world, owner, "select") != selectEntity || htmlFocusableByID(t, world, owner, "editor") != editorEntity {
		t.Fatal("authored attribute update replaced controls")
	}
	field, _ := ecs.Get[TextField](world, fieldEntity)
	toggle, _ := ecs.Get[Toggle[bool]](world, checkEntity)
	editor, _ := ecs.Get[TextField](world, editorEntity)
	runtime, _ := ecs.Get[htmlSelectorRuntime](world, selectEntity)
	item, _ := ecs.Get[SelectorItem](world, runtime.Items[0])
	if field.Placeholder != "After" || field.MaxRunes != 4 || field.Value != "New" || !toggle.Value || editor.Value != "After" || item.Label != "After" {
		t.Fatalf("stale authored values: field=%+v toggle=%+v editor=%+v item=%+v", field, toggle, editor, item)
	}
}

func TestHTMLTextReparentUpdatesRuntimeRelations(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<button id="first"><span id="label">Label</span></button><button id="second">Second</button>`)
	html.Width, html.Height, html.Font = 320, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncHTML(world); err != nil {
		t.Fatal(err)
	}
	second := htmlControlByID(t, world, owner, "second")
	label, _ := htmlui.NodeByID(world, owner, "label")
	destination, _ := htmlui.NodeByID(world, owner, "second")
	label.Remove()
	destination.Append(label)
	if err := SyncHTML(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for entity, access := range ecs.Query2[htmlui.ControlText, scene.Text2D](world).All {
		link := access.A.Get()
		text := access.B.Get()

		if strings.Contains(text.Value, "Label") {
			relation, _ := ecs.Get[Relation[Control]](world, entity)
			if link.Control != second || relation.Owner != second {
				t.Fatalf("text kept old control relation: link=%+v relation=%+v", link, relation)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("reparented label disappeared")
	}
}

func TestHTMLLocalScrollTextUpdateKeepsOffsetsAcrossFrames(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div id="viewport" style="width:200px;height:40px;overflow:auto"><div id="target">Before</div><div style="height:100px">Unchanged</div></div>`)
	html.Width, html.Height, html.Font = 300, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	target, ok := htmlui.NodeByID(world, owner, "target")
	if !ok {
		t.Fatal("missing target")
	}
	var area, unchanged ecs.Entity
	for entity, access := range ecs.Query2[scene.Text2D, htmlui.ScrollContent](world).All {
		text := access.A.Get()
		content := access.B.Get()

		if text.Value == "Unchanged" {
			area, unchanged = content.Area, entity
		}
	}
	if area == 0 {
		t.Fatal("missing scroll content")
	}
	scroll := ecs.GetMut[scene.UIScroll](world, area)
	scroll.Position[1] = -10
	for _, value := range []string{"After", "Before", "After"} {
		if err := SyncAfterLayoutSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		target.SetText(value)
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := SyncAfterLayoutSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		scroll, _ := ecs.Get[scene.UIScroll](world, area)
		if scroll.Position[1] != -10 || !world.IsAlive(unchanged) {
			t.Fatal("local update lost scroll state or sibling")
		}
		for _, access := range ecs.Query2[htmlui.ScrollContent, scene.UITransform](world).All {
			content := access.A.Get()
			transform := access.B.Get()

			if content.Area == area && transform.Translation.Y.Value != content.Y-10 {
				t.Fatal("scroll offset accumulated or was omitted")
			}
		}
	}
}

func TestHTMLLocalControlResizeKeepsTextFieldSelection(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div><input id="target" value="Value"/></div><div><input id="other" value="Other"/></div>`)
	html.Width, html.Height = 320, 200
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "target")
	other := htmlControlByID(t, world, owner, "other")
	htmlui.SetControlValue(world, entity, "Typed")
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control := ecs.GetMut[htmlui.Control](world, entity)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	field := ecs.GetMut[TextField](world, entity)
	field.Anchor, field.Caret = 1, 3
	previous, _ := ecs.Get[htmlui.Control](world, other)
	target, _ := htmlui.NodeByID(world, owner, "target")
	for _, css := range []string{"width:180px", "width:120px"} {
		target.SetAttribute("style", css)
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		field = ecs.GetMut[TextField](world, entity)
		after, _ := ecs.Get[htmlui.Control](world, other)
		if field.Value != "Typed" || field.Anchor != 1 || field.Caret != 3 || after.RenderRevision != previous.RenderRevision {
			t.Fatal("local resize reset TextField state or rewrote unrelated control")
		}
	}
}

func TestHTMLExternalBindingSurvivesScrollFramesAndLocalResize(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div><input id="target" value="Value"/></div>`)
	html.Width, html.Height = 320, 200
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "target")
	base, _ := ecs.Get[scene.UITransform](world, entity)
	areaBundle := ecs.NewBundle(1)
	scroll := scene.UIScroll{Vertical: true}
	scroll.Position[1] = -7
	ecs.Put(&areaBundle, scroll)
	area := world.Spawn(areaBundle)
	htmlui.SetNodeScroll(world, owner, "target", area, 0, 13)
	htmlui.SetNodeOffset(world, owner, "target", 0, 11)
	target, _ := htmlui.NodeByID(world, owner, "target")
	for _, css := range []string{"width:180px", "width:120px", "width:120px", "width:180px"} {
		target.SetAttribute("style", css)
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := SyncAfterLayoutSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		content, _ := ecs.Get[htmlui.ScrollContent](world, entity)
		transform, _ := ecs.Get[scene.UITransform](world, entity)
		parent, _ := ecs.Get[scene.Parent](world, entity)
		if content.Area != area || parent.Entity != area || content.Y != base.Translation.Y.Value-2 || transform.Translation.Y.Value != content.Y-7 {
			t.Fatalf("scroll frame lost bound coordinates: content=%+v transform=%+v parent=%v", content, transform, parent)
		}
	}
}

func TestHTMLStructureKeepsUnchangedTextFieldState(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div style="height:40px"><div id="remove">Remove</div></div><div><input id="field" value="Value"/></div><div><select id="select"><option>A</option></select></div>`)
	html.Width, html.Height = 320, 200
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "field")
	htmlui.SetControlValue(world, entity, "Typed")
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := ecs.GetMut[TextField](world, entity)
	field.Value, field.Anchor, field.Caret = "Typed", 1, 3
	control := ecs.GetMut[htmlui.Control](world, entity)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	revision := control.RenderRevision
	selectEntity := htmlControlByID(t, world, owner, "select")
	selectControl := ecs.GetMut[htmlui.Control](world, selectEntity)
	selectControl.Open = true
	selectRevision := selectControl.RenderRevision
	selector := ecs.GetMut[Selector](world, selectEntity)
	selector.Open = true
	focus, _ := ecs.Get[htmlui.Focusable](world, entity)
	removed, _ := htmlui.NodeByID(world, owner, "remove")
	removed.Remove()
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field = ecs.GetMut[TextField](world, entity)
	after, _ := ecs.Get[htmlui.Control](world, entity)
	nextFocus, _ := ecs.Get[htmlui.Focusable](world, entity)
	selector = ecs.GetMut[Selector](world, selectEntity)
	selectControl = ecs.GetMut[htmlui.Control](world, selectEntity)
	selectStack, _ := ecs.Get[scene.UIStackIndex](world, selectEntity)
	if selector.StackIndex != int(selectStack) || !selector.Open || !selectControl.Open || selectControl.RenderRevision != selectRevision {
		t.Fatal("selector order cache or live state changed incorrectly")
	}
	if field.Value != "Typed" || field.Anchor != 1 || field.Caret != 3 || after.RenderRevision != revision || nextFocus.Order >= focus.Order {
		t.Fatalf("structural update: value=%q selection=%d:%d revision=%d/%d order=%d/%d", field.Value, field.Anchor, field.Caret, after.RenderRevision, revision, nextFocus.Order, focus.Order)
	}
}

func TestHTMLStructuralScrollRangeAppliesAfterLayout(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div id="scroller" style="overflow:auto;height:40px"><div id="remove" style="height:200px">Tall</div><div style="position:absolute;top:0"><input id="field" value="Value"/></div></div>`)
	html.Width, html.Height = 320, 240
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "field")
	content, ok := ecs.Get[htmlui.ScrollContent](world, field)
	if !ok {
		t.Fatal("missing clipped input")
	}
	area := content.Area
	scroll := ecs.GetMut[scene.UIScroll](world, area)
	scroll.Position[1] = -100
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	removed, _ := htmlui.NodeByID(world, owner, "remove")
	removed.Remove()
	for range 3 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := SyncAfterLayoutSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		scroll, _ := ecs.Get[scene.UIScroll](world, area)
		if scroll.Position[1] != 0 || scroll.ContentSize[1] != 40 {
			t.Fatalf("range did not shrink: %+v", scroll)
		}
		for _, access := range ecs.Query2[htmlui.ScrollContent, scene.UITransform](world).All {
			content := access.A.Get()
			transform := access.B.Get()

			if content.Area == area && (transform.Translation.X.Value != content.X+scroll.Position[0] || transform.Translation.Y.Value != content.Y+scroll.Position[1]) {
				t.Fatal("scroll frame used stale content coordinates")
			}
		}
	}
}

func TestHTMLClipMembershipKeepsLiveInputAcrossScrollFrames(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div style="height:200px"><div id="clip" style="overflow:auto;height:40px"><div style="height:100px"><input id="field" value="Value"/></div></div></div>`)
	html.Width, html.Height = 320, 240
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "field")
	htmlui.SetControlValue(world, entity, "Typed")
	field := ecs.GetMut[TextField](world, entity)
	field.Value, field.Anchor, field.Caret = "Typed", 1, 3
	control := ecs.GetMut[htmlui.Control](world, entity)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	clip, _ := htmlui.NodeByID(world, owner, "clip")
	content, _ := ecs.Get[htmlui.ScrollContent](world, entity)
	oldArea := content.Area
	scroll := ecs.GetMut[scene.UIScroll](world, oldArea)
	scroll.Position[1] = -10
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	for _, height := range []string{"0px", "40px", "0px", "40px"} {
		clip.SetAttribute("style", "overflow:auto;height:"+height)
		for range 2 {
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if err := SyncAfterLayoutSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			field, _ := ecs.Get[TextField](world, entity)
			if field.Value != "Typed" || field.Anchor != 1 || field.Caret != 3 {
				t.Fatal("clip membership reset live input")
			}
			parent, _ := ecs.Get[scene.Parent](world, entity)
			transform, _ := ecs.Get[scene.UITransform](world, entity)
			content, attached := ecs.Get[htmlui.ScrollContent](world, entity)
			if height == "0px" {
				if attached || parent.Entity != owner || world.IsAlive(oldArea) {
					t.Fatal("removed clip still owns input")
				}
			} else {
				if !attached || parent.Entity != content.Area || !world.IsAlive(content.Area) {
					t.Fatal("new clip did not own input")
				}
				position, _ := scene.GetUIScrollOffset(world, content.Area)
				if transform.Translation.X.Value != content.X+position[0] || transform.Translation.Y.Value != content.Y+position[1] {
					t.Fatal("scroll frame used stale base")
				}
				oldArea = content.Area
			}
		}
	}
}

func TestHTMLOverflowModeKeepsLiveInputAcrossScrollFrames(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div style="height:200px"><div id="clip" style="overflow:auto;height:40px"><div style="height:100px"><input id="field" value="Value"/></div></div></div>`)
	html.Width, html.Height = 320, 240
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "field")
	htmlui.SetControlValue(world, entity, "Typed")
	field := ecs.GetMut[TextField](world, entity)
	field.Value, field.Anchor, field.Caret = "Typed", 1, 3
	control := ecs.GetMut[htmlui.Control](world, entity)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	clip, _ := htmlui.NodeByID(world, owner, "clip")
	content, _ := ecs.Get[htmlui.ScrollContent](world, entity)
	oldArea := content.Area
	scroll := ecs.GetMut[scene.UIScroll](world, oldArea)
	scroll.Position[1] = -10
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"hidden", "auto", "clip", "scroll", "visible", "auto"} {
		clip.SetAttribute("style", "height:40px;overflow:"+mode)
		for range 2 {
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if err := SyncAfterLayoutSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			field, _ := ecs.Get[TextField](world, entity)
			if field.Value != "Typed" || field.Anchor != 1 || field.Caret != 3 {
				t.Fatal("clip membership reset live input")
			}
			parent, _ := ecs.Get[scene.Parent](world, entity)
			transform, _ := ecs.Get[scene.UITransform](world, entity)
			content, attached := ecs.Get[htmlui.ScrollContent](world, entity)
			if mode == "visible" {
				if attached || parent.Entity != owner || world.IsAlive(oldArea) {
					t.Fatal("removed clip still owns input")
				}
			} else if mode == "hidden" || mode == "clip" {
				if attached || parent.Entity != oldArea || !world.IsAlive(oldArea) || ecs.Has[scene.UIScroll](world, oldArea) {
					t.Fatal("non-scrolling clip retained stale scroll state")
				}
				if transform.Translation.Y.Value != 0 {
					t.Fatal("old scroll offset survived mode change")
				}
			} else {
				if !attached || parent.Entity != content.Area || !world.IsAlive(content.Area) {
					t.Fatal("new clip did not own input")
				}
				position, _ := scene.GetUIScrollOffset(world, content.Area)
				if transform.Translation.X.Value != content.X+position[0] || transform.Translation.Y.Value != content.Y+position[1] {
					t.Fatal("scroll frame used stale base")
				}
				oldArea = content.Area
			}
		}
	}
}

func TestHTMLReparentKeepsLiveInputAcrossScrollFrames(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div id="left" style="height:40px;overflow:auto"><div id="moved" style="height:100px"><input id="field" value="Value"/></div></div><div id="right" style="height:40px;overflow:auto"></div>`)
	html.Width, html.Height = 320, 240
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "field")
	htmlui.SetControlValue(world, entity, "Typed")
	field := ecs.GetMut[TextField](world, entity)
	field.Value, field.Anchor, field.Caret = "Typed", 1, 3
	control := ecs.GetMut[htmlui.Control](world, entity)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	moved, _ := htmlui.NodeByID(world, owner, "moved")
	for _, id := range []string{"right", "left", "right", "left"} {
		content, _ := ecs.Get[htmlui.ScrollContent](world, entity)
		oldArea := content.Area
		scroll := ecs.GetMut[scene.UIScroll](world, oldArea)
		scroll.Position[1] = -10
		if err := SyncAfterLayoutSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		destination, _ := htmlui.NodeByID(world, owner, id)
		moved.Remove()
		destination.Append(moved)
		for range 2 {
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if err := SyncAfterLayoutSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			field, _ := ecs.Get[TextField](world, entity)
			if htmlControlByID(t, world, owner, "field") != entity || field.Value != "Typed" || field.Anchor != 1 || field.Caret != 3 {
				t.Fatal("move reset live input")
			}
			content, ok := ecs.Get[htmlui.ScrollContent](world, entity)
			parent, _ := ecs.Get[scene.Parent](world, entity)
			if !ok || content.Area == oldArea || parent.Entity != content.Area || !world.IsAlive(content.Area) {
				t.Fatal("move retained old scroll parent")
			}
			transform, _ := ecs.Get[scene.UITransform](world, entity)
			position, _ := scene.GetUIScrollOffset(world, content.Area)
			if transform.Translation.X.Value != content.X+position[0] || transform.Translation.Y.Value != content.Y+position[1] {
				t.Fatal("move used stale scroll base")
			}
		}
	}
}

func TestHTMLRetiredSourceKeepsLiveInputAcrossScrollFrames(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<div id="left" style="height:40px;overflow:auto"><div id="moved" style="height:100px"><input id="field" value="Value"/></div></div><div id="right" style="height:40px;overflow:auto"></div>`)
	html.Width, html.Height = 320, 240
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlControlByID(t, world, owner, "field")
	htmlui.SetControlValue(world, entity, "Typed")
	field := ecs.GetMut[TextField](world, entity)
	field.Value, field.Anchor, field.Caret = "Typed", 1, 3
	control := ecs.GetMut[htmlui.Control](world, entity)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	moved, _ := htmlui.NodeByID(world, owner, "moved")
	for _, id := range []string{"right"} {
		content, _ := ecs.Get[htmlui.ScrollContent](world, entity)
		oldArea := content.Area
		scroll := ecs.GetMut[scene.UIScroll](world, oldArea)
		scroll.Position[1] = -10
		if err := SyncAfterLayoutSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		destination, _ := htmlui.NodeByID(world, owner, id)
		moved.Remove()
		source, _ := htmlui.NodeByID(world, owner, "left")
		source.Remove()
		destination.Append(moved)
		for range 2 {
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if err := SyncAfterLayoutSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if world.IsAlive(oldArea) {
				t.Fatal("retired source scroll area is alive")
			}
			field, _ := ecs.Get[TextField](world, entity)
			if htmlControlByID(t, world, owner, "field") != entity || field.Value != "Typed" || field.Anchor != 1 || field.Caret != 3 {
				t.Fatal("move reset live input")
			}
			content, ok := ecs.Get[htmlui.ScrollContent](world, entity)
			parent, _ := ecs.Get[scene.Parent](world, entity)
			if !ok || content.Area == oldArea || parent.Entity != content.Area || !world.IsAlive(content.Area) {
				t.Fatal("move retained old scroll parent")
			}
			transform, _ := ecs.Get[scene.UITransform](world, entity)
			position, _ := scene.GetUIScrollOffset(world, content.Area)
			if transform.Translation.X.Value != content.X+position[0] || transform.Translation.Y.Value != content.Y+position[1] {
				t.Fatal("move used stale scroll base")
			}
		}
	}
}
