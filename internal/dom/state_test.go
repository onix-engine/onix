package dom

import "testing"

func TestElementStatesMatchHTMLFormSemantics(t *testing.T) {
	doc, err := Parse(`<div id="plain" disabled></div><fieldset disabled><legend><input id="legend"></legend><input id="field"></fieldset><select><optgroup disabled><option id="option">One</option></optgroup></select><select disabled><option id="select-option">Two</option></select>`)
	if err != nil {
		t.Fatal(err)
	}
	plain := findNodeByID(doc, "plain")
	legend := findNodeByID(doc, "legend")
	field := findNodeByID(doc, "field")
	option := findNodeByID(doc, "option")
	selectOption := findNodeByID(doc, "select-option")
	if IsDisabled(plain) {
		t.Fatal("a div with a disabled attribute matched :disabled")
	}
	if IsDisabled(legend) {
		t.Fatal("a control in the first legend inherited fieldset disabledness")
	}
	if !IsDisabled(field) {
		t.Fatal("a fieldset descendant did not inherit disabledness")
	}
	if !IsDisabled(option) {
		t.Fatal("an option did not inherit optgroup disabledness")
	}
	if !IsDisabled(selectOption) {
		t.Fatal("an option did not inherit select disabledness")
	}
}

func TestElementStatesSeparateLiveAndAuthoredCheckedness(t *testing.T) {
	doc, err := Parse(`<input id="check" type="checkbox" checked>`)
	if err != nil {
		t.Fatal(err)
	}
	check := findNodeByID(doc, "check")
	check.States.Set(StateChecked, false)
	if IsChecked(check) {
		t.Fatal("live uncheckedness did not override the checked attribute")
	}
	if _, authored := check.Attr("checked"); !authored {
		t.Fatal("changing live checkedness removed the authored attribute")
	}
	delete(check.Attributes, "checked")
	check.States.Set(StateChecked, true)
	if !IsChecked(check) {
		t.Fatal("live checkedness required a checked attribute")
	}
}

func TestElementStatesUseLastSelectedOptionAndCheckedPseudoState(t *testing.T) {
	doc, err := Parse(`<select><option id="first" selected>First</option><option id="last" selected>Last</option></select>`)
	if err != nil {
		t.Fatal(err)
	}
	first := findNodeByID(doc, "first")
	last := findNodeByID(doc, "last")
	if IsChecked(first) || !IsChecked(last) {
		t.Fatalf("option checkedness = first:%v last:%v", IsChecked(first), IsChecked(last))
	}
	if _, ok := first.Attr("selected"); !ok {
		t.Fatal("selection normalization changed the authored selected attribute")
	}
}

func TestEmptyPlaceholderStillProducesPlaceholderShownState(t *testing.T) {
	doc, err := Parse(`<input id="empty" placeholder=""><input id="spaces" placeholder="   ">`)
	if err != nil {
		t.Fatal(err)
	}
	if !IsPlaceholderShown(findNodeByID(doc, "empty")) || !IsPlaceholderShown(findNodeByID(doc, "spaces")) {
		t.Fatal("empty or whitespace placeholder did not match :placeholder-shown")
	}
}

func TestSelectInitializationMatchesChromium(t *testing.T) {
	doc, err := Parse(`<select id="disabled-first"><option disabled>A</option><option>B</option></select><select id="disabled-select" disabled><option>A</option><option>B</option></select><select id="list" size="2"><option>A</option><option>B</option></select><select id="multiple" multiple><option>A</option><option>B</option></select><select id="authored"><option selected>A</option><option selected>B</option></select>`)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id   string
		want int
	}{
		{id: "disabled-first", want: 1},
		{id: "disabled-select", want: 0},
		{id: "list", want: -1},
		{id: "multiple", want: -1},
		{id: "authored", want: 1},
	}
	for _, test := range tests {
		if got := SelectedIndex(findNodeByID(doc, test.id)); got != test.want {
			t.Errorf("%s selected index = %d, want %d", test.id, got, test.want)
		}
	}
}

func TestSelectedAttributeUsesPerOptionDirtyness(t *testing.T) {
	doc, err := Parse(`<select id="mode"><option id="first">A</option><option id="second">B</option></select>`)
	if err != nil {
		t.Fatal(err)
	}
	selectNode := findNodeByID(doc, "mode")
	first := findNodeByID(doc, "first")
	second := findNodeByID(doc, "second")
	SetSelectedIndex(selectNode, 1)

	first.Attributes["selected"] = ""
	SynchronizeSelectedAttribute(first, true)
	if got := SelectedIndex(selectNode); got != 0 {
		t.Fatalf("non-dirty option selected attribute produced index %d, want 0", got)
	}

	second.Attributes["selected"] = ""
	SynchronizeSelectedAttribute(second, true)
	if got := SelectedIndex(selectNode); got != 0 {
		t.Fatalf("dirty option selected attribute changed live index to %d", got)
	}
	if got := DefaultSelectedIndex(selectNode); got != 1 {
		t.Fatalf("authored default selected index = %d, want 1", got)
	}
}

func TestMultipleSelectTracksIndependentSelectedOptions(t *testing.T) {
	doc, err := Parse(`<select id="mode" multiple><option id="first" selected>A</option><option id="second">B</option><option id="third" selected>C</option></select>`)
	if err != nil {
		t.Fatal(err)
	}
	mode := findNodeByID(doc, "mode")
	first := findNodeByID(doc, "first")
	second := findNodeByID(doc, "second")
	third := findNodeByID(doc, "third")
	if !IsChecked(first) || IsChecked(second) || !IsChecked(third) || SelectedIndex(mode) != 0 {
		t.Fatalf("initial multiple selection = first:%v second:%v third:%v index:%d", IsChecked(first), IsChecked(second), IsChecked(third), SelectedIndex(mode))
	}
	if !SetOptionSelectedAt(mode, 1, true) || !SetOptionSelectedAt(mode, 2, false) {
		t.Fatal("failed to update multiple selectedness")
	}
	if !IsChecked(first) || !IsChecked(second) || IsChecked(third) || SelectedIndex(mode) != 0 {
		t.Fatalf("live multiple selection = first:%v second:%v third:%v index:%d", IsChecked(first), IsChecked(second), IsChecked(third), SelectedIndex(mode))
	}
}

func TestSetSelectedRangeSkipsDisabledOptionsInBatch(t *testing.T) {
	doc, err := Parse(`<select id="mode" multiple><option selected>A</option><option>B</option><option disabled selected>C</option><optgroup disabled><option selected>D</option></optgroup><option>E</option></select>`)
	if err != nil {
		t.Fatal(err)
	}
	mode := findNodeByID(doc, "mode")
	if !SetSelectedRange(mode, 1, 4) {
		t.Fatal("set selected range failed")
	}
	wants := [...]bool{false, true, true, true, true}
	for index, want := range wants {
		optionIndex := 0
		option := optionAt(mode, index, &optionIndex)
		if got := IsChecked(option); got != want {
			t.Fatalf("option %d selected = %v, want %v", index, got, want)
		}
	}
}

func TestSelectMultipleAndSizeAttributeNormalization(t *testing.T) {
	doc, err := Parse(`<select id="multiple" multiple><option id="a" selected>A</option><option id="b" selected>B</option><option id="c" selected>C</option></select><select id="list" size="2"><option id="first">A</option><option>B</option></select>`)
	if err != nil {
		t.Fatal(err)
	}
	multiple := findNodeByID(doc, "multiple")
	delete(multiple.Attributes, "multiple")
	SynchronizeSelectAttribute(multiple, "multiple")
	if !IsChecked(findNodeByID(doc, "a")) || IsChecked(findNodeByID(doc, "b")) || IsChecked(findNodeByID(doc, "c")) {
		t.Fatal("removing multiple did not retain only the first selection")
	}
	list := findNodeByID(doc, "list")
	if SelectedIndex(list) != -1 {
		t.Fatalf("initial list selection = %d, want -1", SelectedIndex(list))
	}
	delete(list.Attributes, "size")
	SynchronizeSelectAttribute(list, "size")
	if SelectedIndex(list) != 0 {
		t.Fatalf("default-size selection = %d, want 0", SelectedIndex(list))
	}
}

func TestSelectChildChangesNormalizeLiveSelection(t *testing.T) {
	for _, test := range []struct {
		name, attributes string
		multiple         bool
		fallback         int
	}{
		{name: "dropdown", fallback: 1},
		{name: "listbox", attributes: `size="2"`, fallback: -1},
		{name: "multiple", attributes: "multiple", multiple: true, fallback: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Parse(`<select id="mode" ` + test.attributes + `><option disabled>A</option><option>B</option></select>`)
			if err != nil {
				t.Fatal(err)
			}
			mode := findNodeByID(doc, "mode")
			SetSelectedIndex(mode, 1)
			previous := mode.Children[1]
			group := NewElement("optgroup", nil)
			option := NewElement("option", map[string]string{"selected": "", "disabled": ""})
			group.AppendChild(option)
			mode.AppendChild(group)
			SynchronizeSelectChildren(group)
			if !IsChecked(option) || IsChecked(previous) != test.multiple {
				t.Fatal("inserted selected option did not normalize existing selection")
			}
			if dirty, _ := previous.States.Get(StateDirtySelected); !dirty {
				t.Fatal("structural change cleared existing dirtyness")
			}
			if dirty, _ := option.States.Get(StateDirtySelected); dirty {
				t.Fatal("structural change dirtied the inserted option")
			}
			SetSelectedIndex(mode, 2)
			mode.Children = mode.Children[:2]
			group.Parent = nil
			SynchronizeSelectChildren(mode)
			if got := SelectedIndex(mode); got != test.fallback {
				t.Fatalf("selection after removal = %d, want %d", got, test.fallback)
			}
			if allocations := testing.AllocsPerRun(100, func() { SynchronizeSelectChildren(mode) }); allocations != 0 {
				t.Fatalf("selection normalization allocated %g times", allocations)
			}
		})
	}
}

func TestResetFormRestoresDefaultsAndClearsDirtyState(t *testing.T) {
	doc, err := Parse(`<form id="settings"><input id="field" value="default" placeholder=""><input id="check" type="checkbox" checked><textarea id="notes">initial</textarea><select id="mode" multiple><option id="first" selected>A</option><option id="second">B</option></select></form><input id="external" form="settings" value="outside">`)
	if err != nil {
		t.Fatal(err)
	}
	form := findNodeByID(doc, "settings")
	field := findNodeByID(doc, "field")
	check := findNodeByID(doc, "check")
	notes := findNodeByID(doc, "notes")
	mode := findNodeByID(doc, "mode")
	first := findNodeByID(doc, "first")
	second := findNodeByID(doc, "second")
	external := findNodeByID(doc, "external")
	SetLiveValue(field, "", true)
	SetLiveValue(notes, "edited", true)
	SetLiveValue(external, "changed", true)
	check.States.Set(StateChecked, false)
	check.States.Set(StateDirtyChecked, true)
	SetOptionSelectedAt(mode, 0, false)
	SetOptionSelectedAt(mode, 1, true)

	if !ResetForm(form) {
		t.Fatal("form reset failed")
	}
	if IsPlaceholderShown(field) || !IsChecked(check) || !IsChecked(first) || IsChecked(second) {
		t.Fatalf("reset state = placeholder:%v check:%v first:%v second:%v", IsPlaceholderShown(field), IsChecked(check), IsChecked(first), IsChecked(second))
	}
	for _, node := range []*Node{field, notes, external} {
		if dirty, _ := node.States.Get(StateDirtyValue); dirty {
			t.Fatalf("dirty value remained on %s", node.Data)
		}
	}
	if dirty, _ := check.States.Get(StateDirtyChecked); dirty {
		t.Fatal("dirty checkedness remained after reset")
	}
	if HasDirtySelectedOption(mode) {
		t.Fatal("dirty selectedness remained after reset")
	}

	allocations := testing.AllocsPerRun(100, func() {
		if !ResetForm(form) {
			panic("form reset failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("form reset allocated %.0f objects, want 0", allocations)
	}
}

func TestDirtyValueKeepsLivePlaceholderState(t *testing.T) {
	doc, err := Parse(`<input id="field" value="default" placeholder="">`)
	if err != nil {
		t.Fatal(err)
	}
	field := findNodeByID(doc, "field")
	SetLiveValue(field, "", true)
	field.Attributes["value"] = "new default"
	SynchronizeValueAttribute(field)
	if !IsPlaceholderShown(field) {
		t.Fatal("dirty empty value was replaced by a non-empty value attribute")
	}
	delete(field.Attributes, "placeholder")
	SynchronizePlaceholderAttribute(field)
	if IsPlaceholderShown(field) {
		t.Fatal("removed placeholder attribute still matched :placeholder-shown")
	}
}

func TestRadioGroupUsesExplicitFormOwnerAndDirtyCheckedness(t *testing.T) {
	doc, err := Parse(`<form id="left"></form><form id="right"></form><input id="first" type="radio" name="group" form="left" checked><input id="second" type="radio" name="group" form="left" checked><input id="other" type="radio" name="group" form="right" checked>`)
	if err != nil {
		t.Fatal(err)
	}
	first := findNodeByID(doc, "first")
	second := findNodeByID(doc, "second")
	other := findNodeByID(doc, "other")
	if IsChecked(first) || !IsChecked(second) || !IsChecked(other) {
		t.Fatalf("radio states = first:%v second:%v other:%v", IsChecked(first), IsChecked(second), IsChecked(other))
	}
	dirty, _ := first.States.Get(StateDirtyChecked)
	if !dirty {
		t.Fatal("radio unchecked by group normalization was not marked dirty")
	}
	delete(first.Attributes, "checked")
	SynchronizeCheckedAttribute(first, false)
	first.Attributes["checked"] = ""
	SynchronizeCheckedAttribute(first, true)
	if IsChecked(first) {
		t.Fatal("checked attribute replaced dirty radio checkedness")
	}
}

func TestTabIndexAndContentEditableFocusability(t *testing.T) {
	doc, err := Parse(`<div id="plain"></div><div id="negative" tabindex="-1"></div><div id="invalid" tabindex="nope"></div><div id="editable" contenteditable><span id="nested" contenteditable>Nested</span><span id="blocked" contenteditable="false">Blocked</span></div>`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := TabIndex(findNodeByID(doc, "plain")); ok {
		t.Fatal("missing tabindex parsed as explicit")
	}
	if index, ok := TabIndex(findNodeByID(doc, "negative")); !ok || index != -1 {
		t.Fatalf("negative tabindex = %d, %v", index, ok)
	}
	if _, ok := TabIndex(findNodeByID(doc, "invalid")); ok {
		t.Fatal("invalid tabindex became focusable")
	}
	editable := findNodeByID(doc, "editable")
	nested := findNodeByID(doc, "nested")
	blocked := findNodeByID(doc, "blocked")
	if !IsEditingHost(editable) || IsEditingHost(nested) || IsContentEditable(blocked) {
		t.Fatalf("editable states = host:%v nested-host:%v blocked:%v", IsEditingHost(editable), IsEditingHost(nested), IsContentEditable(blocked))
	}
}

func findNodeByID(node *Node, id string) *Node {
	if node == nil {
		return nil
	}
	if value, ok := node.Attr("id"); ok && value == id {
		return node
	}
	for _, child := range node.Children {
		if found := findNodeByID(child, id); found != nil {
			return found
		}
	}
	return nil
}
