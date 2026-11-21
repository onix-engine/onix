package element

import (
	"strconv"
	"strings"
	"sync"

	uinode "github.com/onix-engine/onix/scene/node"
)

const (
	StateChecked = 1 << iota
	StatePlaceholderShown
	StateIndeterminate
	StateDirtyChecked
	StateValueEmpty
	StateDirtyValue
	StateDirtySelected
)

func InitializeElementStates(root *uinode.Node) {
	InitializeDocument(root, nil, nil)
}

// InitializeDocument sets document order and form state. Visit observes each
// retained node in preorder. Keep can inspect and exclude a child before its
// subtree is initialized. Removed metadata can be consumed by Keep.
func InitializeDocument(root *uinode.Node, visit func(*uinode.Node), keep func(*uinode.Node) bool) {
	var order uint32
	var radios *radioDefaults
	var parentForm *uinode.Node
	if root != nil {
		parentForm = ancestorTag(root, "form")
	}
	initializeElementStates(root, &order, visit, keep, parentForm, &radios)
	if radios != nil {
		radios.finish(root)
	}
}

func HasInputType(node *uinode.Node, inputType string) bool {
	if node == nil || node.Type != uinode.ElementNode || node.Data != "input" {
		return false
	}
	typeValue, ok := node.Attr("type")
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(typeValue), inputType)
}

type checkedRadio struct {
	form *uinode.Node
	node *uinode.Node
	name string
}

type radioGroup struct {
	form *uinode.Node
	name string
}

type radioDefaults struct {
	forms   map[string]*uinode.Node
	winners map[radioGroup]*uinode.Node
	checked []checkedRadio
}

var radioDefaultsPool = sync.Pool{New: func() any { return new(radioDefaults) }}

func (r *radioDefaults) finish(root *uinode.Node) {
	if len(r.checked) > 1 {
		r.normalize(root)
	}
	// Drop oversized scratch rather than retaining a whole imported document's peak.
	if max(cap(r.checked), len(r.forms)) > 4096 {
		return
	}
	clear(r.checked)
	r.checked = r.checked[:0]
	clear(r.forms)
	clear(r.winners)
	radioDefaultsPool.Put(r)
}

func (r *radioDefaults) normalize(root *uinode.Node) {
	if r.winners == nil {
		r.winners = make(map[radioGroup]*uinode.Node)
	}
	indexed := false
	for _, radio := range r.checked {
		owner := radio.form
		if id, explicit := radio.node.Attr("form"); explicit {
			if !indexed {
				if r.forms == nil {
					r.forms = make(map[string]*uinode.Node)
				}
				indexForms(treeRoot(root), r.forms)
				indexed = true
			}
			owner = r.forms[id]
		}
		key := radioGroup{form: owner, name: radio.name}
		if previous := r.winners[key]; previous != nil {
			previous.States.Set(StateChecked, false)
			previous.States.Set(StateDirtyChecked, true)
			// Each originally checked member was unchecked by another group member
			// during normalization, including the final winner.
			radio.node.States.Set(StateDirtyChecked, true)
		}
		r.winners[key] = radio.node
	}
}

func indexForms(node *uinode.Node, forms map[string]*uinode.Node) {
	if node == nil {
		return
	}
	if node.Type == uinode.ElementNode && node.Data == "form" {
		if id, _ := node.Attr("id"); id != "" && forms[id] == nil {
			forms[id] = node
		}
	}
	for _, child := range node.Children {
		indexForms(child, forms)
	}
}

func clearCheckedRadio(node, except *uinode.Node, name string) {
	if node == nil {
		return
	}
	if node != except && node.Type == uinode.ElementNode && HasInputType(node, "radio") {
		candidateName, _ := node.Attr("name")
		if candidateName == name && FormOwner(node) == FormOwner(except) && IsChecked(node) {
			node.States.Set(StateChecked, false)
			node.States.Set(StateDirtyChecked, true)
		}
	}
	for _, child := range node.Children {
		clearCheckedRadio(child, except, name)
	}
}

func ancestorTag(node *uinode.Node, tag string) *uinode.Node {
	for node = node.Parent; node != nil; node = node.Parent {
		if node.Type == uinode.ElementNode && node.Data == tag {
			return node
		}
	}
	return nil
}

// FormOwner returns the form associated with a form control. An explicit form
// attribute takes precedence over the nearest ancestor form.
func FormOwner(node *uinode.Node) *uinode.Node {
	if node == nil {
		return nil
	}
	if id, explicit := node.Attr("form"); explicit {
		return findFormByID(treeRoot(node), id)
	}
	return ancestorTag(node, "form")
}

func findFormByID(node *uinode.Node, id string) *uinode.Node {
	if node == nil || id == "" {
		return nil
	}
	if node.Type == uinode.ElementNode && node.Data == "form" {
		if candidate, ok := node.Attr("id"); ok && candidate == id {
			return node
		}
	}
	for _, child := range node.Children {
		if form := findFormByID(child, id); form != nil {
			return form
		}
	}
	return nil
}

func treeRoot(node *uinode.Node) *uinode.Node {
	for node != nil && node.Parent != nil {
		node = node.Parent
	}
	return node
}

func SameRadioGroup(left, right *uinode.Node) bool {
	if left == nil || right == nil || treeRoot(left) != treeRoot(right) || !HasInputType(left, "radio") || !HasInputType(right, "radio") {
		return false
	}
	leftName, _ := left.Attr("name")
	rightName, _ := right.Attr("name")
	return leftName != "" && leftName == rightName && FormOwner(left) == FormOwner(right)
}

// TabIndex parses the global tabindex attribute as a valid signed integer.
func TabIndex(node *uinode.Node) (int, bool) {
	if node == nil || node.Type != uinode.ElementNode {
		return 0, false
	}
	raw, ok := node.Attr("tabindex")
	if !ok {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	return value, err == nil
}

// IsContentEditable resolves the enumerated contenteditable attribute through
// the element's ancestors.
func IsContentEditable(node *uinode.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current.Type != uinode.ElementNode {
			continue
		}
		raw, ok := current.Attr("contenteditable")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "", "true", "plaintext-only":
			return true
		case "false":
			return false
		}
	}
	return false
}

// IsEditingHost reports whether node starts a contenteditable region. Nested
// editable descendants are not independent sequential focus targets.
func IsEditingHost(node *uinode.Node) bool {
	if node == nil || node.Type != uinode.ElementNode || !IsContentEditable(node) {
		return false
	}
	parent := node.Parent
	for parent != nil && parent.Type != uinode.ElementNode {
		parent = parent.Parent
	}
	return !IsContentEditable(parent)
}

func initializeElementStates(node *uinode.Node, order *uint32, visit func(*uinode.Node), keep func(*uinode.Node) bool, form *uinode.Node, radios **radioDefaults) {
	if node == nil {
		return
	}
	node.Order = *order
	*order++
	if visit != nil {
		visit(node)
	}
	if node.Type == uinode.ElementNode {
		switch node.Data {
		case "form":
			form = node
		case "input":
			hasRadio := HasInputType(node, "radio")
			if hasRadio || HasInputType(node, "checkbox") {
				_, checked := node.Attr("checked")
				node.States.Set(StateChecked, checked)
				if hasRadio && checked {
					if name, _ := node.Attr("name"); name != "" {
						if *radios == nil {
							*radios = radioDefaultsPool.Get().(*radioDefaults)
						}
						r := *radios
						r.checked = append(r.checked, checkedRadio{form: form, node: node, name: name})
					}
				}
			}
			value, _ := node.Attr("value")
			SetLiveValue(node, value, false)
		case "option":
			_, selected := node.Attr("selected")
			node.States.Set(StateChecked, selected)
		}
	}
	if keep == nil {
		for _, child := range node.Children {
			initializeElementStates(child, order, visit, nil, form, radios)
		}
	} else {
		node.KeepChildren(func(child *uinode.Node) bool {
			if !keep(child) {
				return false
			}
			initializeElementStates(child, order, visit, keep, form, radios)
			return true
		})
	}
	if node.Type == uinode.ElementNode {
		switch node.Data {
		case "select":
			resetSelectStates(node)
		case "textarea":
			setLiveValueEmpty(node, !hasTextContent(node), false)
		}
	}
}

type selectResetState struct {
	index         int
	firstEnabled  int
	firstAuthored int
	lastAuthored  int
}

func resetSelectStates(node *uinode.Node) {
	state := selectResetState{firstEnabled: -1, firstAuthored: -1, lastAuthored: -1}
	resetSelectOptions(node, &state)
	if _, multiple := node.Attr("multiple"); multiple {
		return
	}
	selected := state.lastAuthored
	if selected < 0 && selectDisplaySize(node) == 1 {
		selected = state.firstEnabled
	}
	setSelectIndex(node, selected, nil)
}

func resetSelectOptions(node *uinode.Node, state *selectResetState) {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			_, authored := child.Attr("selected")
			child.States.Set(StateChecked, authored)
			child.States.Clear(StateDirtySelected)
			if state.firstEnabled < 0 && !optionDisabledForSelection(child) {
				state.firstEnabled = state.index
			}
			if authored {
				if state.firstAuthored < 0 {
					state.firstAuthored = state.index
				}
				state.lastAuthored = state.index
			}
			state.index++
			continue
		}
		resetSelectOptions(child, state)
	}
}

func selectDisplaySize(node *uinode.Node) int {
	raw, ok := node.Attr("size")
	if !ok {
		return 1
	}
	size, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || size <= 0 {
		return 1
	}
	return size
}

// SelectedIndex returns the first live selected option in tree order.
func SelectedIndex(selectNode *uinode.Node) int {
	index, selected := 0, -1
	findSelectedIndex(selectNode, &index, &selected)
	return selected
}

func findSelectedIndex(node *uinode.Node, index, selected *int) bool {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			if IsChecked(child) {
				*selected = *index
				return true
			}
			*index++
			continue
		}
		if findSelectedIndex(child, index, selected) {
			return true
		}
	}
	return false
}

// DefaultSelectedIndex returns the effective selection produced by resetting
// the select from its authored option attributes.
func DefaultSelectedIndex(selectNode *uinode.Node) int {
	state := selectResetState{firstEnabled: -1, firstAuthored: -1, lastAuthored: -1}
	inspectSelectOptions(selectNode, &state)
	if _, multiple := selectNode.Attr("multiple"); multiple {
		return state.firstAuthored
	}
	if state.lastAuthored >= 0 {
		return state.lastAuthored
	}
	if selectDisplaySize(selectNode) == 1 {
		return state.firstEnabled
	}
	return -1
}

func inspectSelectOptions(node *uinode.Node, state *selectResetState) {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			if state.firstEnabled < 0 && !optionDisabledForSelection(child) {
				state.firstEnabled = state.index
			}
			if _, authored := child.Attr("selected"); authored {
				if state.firstAuthored < 0 {
					state.firstAuthored = state.index
				}
				state.lastAuthored = state.index
			}
			state.index++
			continue
		}
		inspectSelectOptions(child, state)
	}
}

// SetSelectedIndex updates live option selectedness. The selected option gets
// a dirty flag; options deselected as a consequence retain their dirtyness.
func SetSelectedIndex(selectNode *uinode.Node, selected int) {
	var dirty *uinode.Node
	index := 0
	dirty = optionAt(selectNode, selected, &index)
	setSelectIndex(selectNode, selected, dirty)
}

// SetOptionSelected updates one option's selectedness as the selected IDL
// property does. A single-select keeps at most one selected option.
func SetOptionSelected(option *uinode.Node, selected bool) bool {
	if option == nil || option.Type != uinode.ElementNode || option.Data != "option" {
		return false
	}
	option.States.Set(StateChecked, selected)
	option.States.Set(StateDirtySelected, true)
	selectNode := ancestorTag(option, "select")
	if selectNode == nil {
		return true
	}
	if _, multiple := selectNode.Attr("multiple"); selected && !multiple {
		unselectOtherOptions(selectNode, option)
		return true
	}
	if !selected {
		ensureSelectSelection(selectNode)
	}
	return true
}

func SetOptionSelectedAt(selectNode *uinode.Node, optionIndex int, selected bool) bool {
	index := 0
	option := optionAt(selectNode, optionIndex, &index)
	return SetOptionSelected(option, selected)
}

// SetSelectedRange applies a user range selection to a multiple select in one
// mutation pass. Disabled options retain their selectedness because keyboard
// and pointer input cannot toggle them.
func SetSelectedRange(selectNode *uinode.Node, start, end int) bool {
	if selectNode == nil || selectNode.Type != uinode.ElementNode || selectNode.Data != "select" || start < 0 || end < 0 {
		return false
	}
	if _, multiple := selectNode.Attr("multiple"); !multiple {
		return false
	}
	if start > end {
		start, end = end, start
	}
	if count := selectOptionCount(selectNode); end >= count {
		return false
	}
	index := 0
	setSelectedRange(selectNode, start, end, &index)
	return true
}

func selectOptionCount(node *uinode.Node) int {
	count := 0
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			count++
			continue
		}
		count += selectOptionCount(child)
	}
	return count
}

func setSelectedRange(node *uinode.Node, start, end int, index *int) {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			if !optionDisabledForSelection(child) {
				selected := *index >= start && *index <= end
				if IsChecked(child) != selected {
					child.States.Set(StateChecked, selected)
					child.States.Set(StateDirtySelected, true)
				}
			}
			*index++
			continue
		}
		setSelectedRange(child, start, end, index)
	}
}

func setSelectIndex(node *uinode.Node, selected int, dirty *uinode.Node) {
	index := 0
	setSelectOptions(node, selected, dirty, &index)
}

func setSelectOptions(node *uinode.Node, selected int, dirty *uinode.Node, index *int) {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			child.States.Set(StateChecked, *index == selected)
			if child == dirty {
				child.States.Set(StateDirtySelected, true)
			}
			*index++
			continue
		}
		setSelectOptions(child, selected, dirty, index)
	}
}

func optionAt(node *uinode.Node, target int, index *int) *uinode.Node {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			if *index == target {
				return child
			}
			*index++
			continue
		}
		if option := optionAt(child, target, index); option != nil {
			return option
		}
	}
	return nil
}

// SynchronizeSelectedAttribute applies an option selected content-attribute
// mutation unless that option's live selectedness is dirty.
func SynchronizeSelectedAttribute(option *uinode.Node, present bool) {
	if option == nil || option.Type != uinode.ElementNode || option.Data != "option" {
		return
	}
	dirty, _ := option.States.Get(StateDirtySelected)
	if dirty {
		return
	}
	option.States.Set(StateChecked, present)
	selectNode := ancestorTag(option, "select")
	if selectNode == nil {
		return
	}
	if _, multiple := selectNode.Attr("multiple"); present && !multiple {
		unselectOtherOptions(selectNode, option)
		return
	}
	if !present {
		ensureSelectSelection(selectNode)
	}
}

// SynchronizeSelectAttribute applies multiple/size mutations to live
// selectedness using the select element's normalization rules.
func SynchronizeSelectAttribute(selectNode *uinode.Node, name string) {
	if selectNode == nil || selectNode.Type != uinode.ElementNode || selectNode.Data != "select" {
		return
	}
	if name == "multiple" {
		if _, multiple := selectNode.Attr("multiple"); multiple {
			return
		}
		keepFirstSelectedOption(selectNode, nil)
	}
	ensureSelectSelection(selectNode)
}

// SynchronizeSelectChildren normalizes live selection after an option list
// changes. Keep the last selected option without resetting defaults or dirtyness.
func SynchronizeSelectChildren(node *uinode.Node) {
	for node != nil && (node.Type != uinode.ElementNode || node.Data != "select") {
		node = node.Parent
	}
	if node == nil {
		return
	}
	if _, multiple := node.Attr("multiple"); multiple {
		return
	}
	var first, selected *uinode.Node
	normalizeSelectChildren(node, &first, &selected)
	if selected == nil && first != nil && selectDisplaySize(node) == 1 {
		first.States.Set(StateChecked, true)
	}
}

func normalizeSelectChildren(node *uinode.Node, first, selected **uinode.Node) {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			if *first == nil && !optionDisabledForSelection(child) {
				*first = child
			}
			if IsChecked(child) {
				if *selected != nil {
					(*selected).States.Set(StateChecked, false)
				}
				*selected = child
			}
			continue
		}
		normalizeSelectChildren(child, first, selected)
	}
}

func keepFirstSelectedOption(node *uinode.Node, selected *uinode.Node) *uinode.Node {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			if IsChecked(child) {
				if selected == nil {
					selected = child
				} else {
					child.States.Set(StateChecked, false)
				}
			}
			continue
		}
		selected = keepFirstSelectedOption(child, selected)
	}
	return selected
}

func unselectOtherOptions(node, selected *uinode.Node) {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			if child != selected {
				child.States.Set(StateChecked, false)
			}
			continue
		}
		unselectOtherOptions(child, selected)
	}
}

func ensureSelectSelection(selectNode *uinode.Node) {
	if _, multiple := selectNode.Attr("multiple"); multiple || selectDisplaySize(selectNode) != 1 || SelectedIndex(selectNode) >= 0 {
		return
	}
	index, firstEnabled := 0, -1
	findFirstEnabledOption(selectNode, &index, &firstEnabled)
	setSelectIndex(selectNode, firstEnabled, nil)
}

func findFirstEnabledOption(node *uinode.Node, index, first *int) bool {
	for _, child := range node.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			if !optionDisabledForSelection(child) {
				*first = *index
				return true
			}
			*index++
			continue
		}
		if findFirstEnabledOption(child, index, first) {
			return true
		}
	}
	return false
}

func HasDirtySelectedOption(selectNode *uinode.Node) bool {
	for _, child := range selectNode.Children {
		if child == nil || child.Type != uinode.ElementNode {
			continue
		}
		if child.Data == "option" {
			dirty, _ := child.States.Get(StateDirtySelected)
			if dirty {
				return true
			}
			continue
		}
		if HasDirtySelectedOption(child) {
			return true
		}
	}
	return false
}

// ResetForm restores controls owned by form to their authored defaults. The
// traversal includes controls associated through an explicit form attribute.
func ResetForm(form *uinode.Node) bool {
	if form == nil || form.Type != uinode.ElementNode || form.Data != "form" {
		return false
	}
	root := treeRoot(form)
	resetFormControls(root, form)
	normalizeResetRadioGroups(root, root, form)
	return true
}

func resetFormControls(node, form *uinode.Node) {
	if node == nil {
		return
	}
	if node.Type == uinode.ElementNode && FormOwner(node) == form {
		switch node.Data {
		case "input":
			node.States.Clear(StateDirtyValue)
			SynchronizeValueAttribute(node)
			if HasInputType(node, "checkbox") || HasInputType(node, "radio") {
				node.States.Clear(StateDirtyChecked)
				_, checked := node.Attr("checked")
				node.States.Set(StateChecked, checked)
			}
		case "textarea":
			node.States.Clear(StateDirtyValue)
			SynchronizeTextContent(node)
		case "select":
			resetSelectStates(node)
		}
	}
	for _, child := range node.Children {
		resetFormControls(child, form)
	}
}

func normalizeResetRadioGroups(root, node, form *uinode.Node) {
	if node == nil {
		return
	}
	if node.Type == uinode.ElementNode && HasInputType(node, "radio") && FormOwner(node) == form {
		name, _ := node.Attr("name")
		_, checked := node.Attr("checked")
		if checked && name != "" {
			clearCheckedRadio(root, node, name)
			node.States.Set(StateChecked, true)
		}
	}
	for _, child := range node.Children {
		normalizeResetRadioGroups(root, child, form)
	}
}

func optionDisabledForSelection(option *uinode.Node) bool {
	if option == nil || option.Type != uinode.ElementNode || option.Data != "option" {
		return true
	}
	if _, disabled := option.Attr("disabled"); disabled {
		return true
	}
	for ancestor := option.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type != uinode.ElementNode {
			continue
		}
		if ancestor.Data == "optgroup" {
			_, disabled := ancestor.Attr("disabled")
			return disabled
		}
		if ancestor.Data == "select" {
			break
		}
	}
	return false
}

func IsChecked(node *uinode.Node) bool {
	if node == nil || node.Type != uinode.ElementNode {
		return false
	}
	switch node.Data {
	case "input":
		if !HasInputType(node, "checkbox") && !HasInputType(node, "radio") {
			return false
		}
	case "option":
	default:
		return false
	}
	checked, resolved := node.States.Get(StateChecked)
	if resolved {
		return checked
	}
	attribute := "checked"
	if node.Data == "option" {
		attribute = "selected"
	}
	_, checked = node.Attr(attribute)
	return checked
}

// SynchronizeCheckedAttribute applies a checked content-attribute mutation to
// live checkedness while the control's dirty checkedness flag is clear.
func SynchronizeCheckedAttribute(node *uinode.Node, present bool) {
	if node == nil || node.Type != uinode.ElementNode || node.Data != "input" || !HasInputType(node, "checkbox") && !HasInputType(node, "radio") {
		return
	}
	dirty, _ := node.States.Get(StateDirtyChecked)
	if dirty {
		return
	}
	if present && HasInputType(node, "radio") {
		name, _ := node.Attr("name")
		if name != "" {
			clearCheckedRadio(treeRoot(node), node, name)
		}
	}
	node.States.Set(StateChecked, present)
}

// IsWidgetControl identifies elements rendered by the native HTML widget path.
func IsWidgetControl(node *uinode.Node) bool {
	if node == nil || node.Type != uinode.ElementNode {
		return false
	}
	switch node.Data {
	case "button", "input", "select", "textarea":
		return true
	default:
		return false
	}
}

func SupportsDisabled(node *uinode.Node) bool {
	if node == nil || node.Type != uinode.ElementNode {
		return false
	}
	switch node.Data {
	case "button", "fieldset", "input", "optgroup", "option", "select", "textarea":
		return true
	default:
		return false
	}
}

func IsDisabled(node *uinode.Node) bool {
	if !SupportsDisabled(node) {
		return false
	}
	if _, disabled := node.Attr("disabled"); disabled {
		return true
	}
	if node.Data == "option" {
		for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Type == uinode.ElementNode && ancestor.Data == "optgroup" {
				if _, disabled := ancestor.Attr("disabled"); disabled {
					return true
				}
			}
			if ancestor.Type == uinode.ElementNode && ancestor.Data == "select" {
				return IsDisabled(ancestor)
			}
		}
	}
	for fieldset := node.Parent; fieldset != nil; fieldset = fieldset.Parent {
		if fieldset.Type != uinode.ElementNode || fieldset.Data != "fieldset" {
			continue
		}
		if _, disabled := fieldset.Attr("disabled"); !disabled {
			continue
		}
		if !withinFirstLegend(node, fieldset) {
			return true
		}
	}
	return false
}

func withinFirstLegend(node, fieldset *uinode.Node) bool {
	var legend *uinode.Node
	for _, child := range fieldset.Children {
		if child != nil && child.Type == uinode.ElementNode && child.Data == "legend" {
			legend = child
			break
		}
	}
	if legend == nil {
		return false
	}
	for current := node; current != nil && current != fieldset; current = current.Parent {
		if current == legend {
			return true
		}
	}
	return false
}

func SupportsPlaceholder(node *uinode.Node) bool {
	if node == nil || node.Type != uinode.ElementNode {
		return false
	}
	if node.Data == "textarea" {
		return true
	}
	if node.Data != "input" {
		return false
	}
	typeValue, _ := node.Attr("type")
	switch strings.ToLower(strings.TrimSpace(typeValue)) {
	case "", "email", "password", "search", "tel", "text", "url":
		return true
	default:
		return false
	}
}

// SetLiveValue updates the compact value state used by :placeholder-shown.
// The string itself remains owned by the generated form control.
func SetLiveValue(node *uinode.Node, value string, dirty bool) {
	setLiveValueEmpty(node, value == "", dirty)
}

func setLiveValueEmpty(node *uinode.Node, empty, dirty bool) {
	if node == nil || node.Type != uinode.ElementNode || node.Data != "input" && node.Data != "textarea" {
		return
	}
	node.States.Set(StateValueEmpty, empty)
	if dirty {
		node.States.Set(StateDirtyValue, true)
	}
	synchronizePlaceholderShown(node)
}

// SynchronizeValueAttribute updates live value state from an input value
// content attribute while the dirty value flag is clear.
func SynchronizeValueAttribute(node *uinode.Node) {
	if node == nil || node.Type != uinode.ElementNode || node.Data != "input" {
		return
	}
	dirty, _ := node.States.Get(StateDirtyValue)
	if dirty {
		return
	}
	value, _ := node.Attr("value")
	setLiveValueEmpty(node, value == "", false)
}

// SynchronizeTextContent updates a textarea's live value state from its child
// text while the dirty value flag is clear.
func SynchronizeTextContent(node *uinode.Node) {
	if node == nil || node.Type != uinode.ElementNode || node.Data != "textarea" {
		return
	}
	dirty, _ := node.States.Get(StateDirtyValue)
	if dirty {
		return
	}
	setLiveValueEmpty(node, !hasTextContent(node), false)
}

func SynchronizePlaceholderAttribute(node *uinode.Node) {
	synchronizePlaceholderShown(node)
}

func synchronizePlaceholderShown(node *uinode.Node) {
	if !SupportsPlaceholder(node) {
		node.States.Set(StatePlaceholderShown, false)
		return
	}
	empty, resolved := node.States.Get(StateValueEmpty)
	if !resolved {
		if node.Data == "textarea" {
			empty = !hasTextContent(node)
		} else {
			value, _ := node.Attr("value")
			empty = value == ""
		}
	}
	_, hasPlaceholder := node.Attr("placeholder")
	node.States.Set(StatePlaceholderShown, hasPlaceholder && empty)
}

func IsPlaceholderShown(node *uinode.Node) bool {
	if !SupportsPlaceholder(node) {
		return false
	}
	shown, resolved := node.States.Get(StatePlaceholderShown)
	if resolved {
		return shown
	}
	_, hasPlaceholder := node.Attr("placeholder")
	if !hasPlaceholder {
		return false
	}
	if node.Data == "textarea" {
		return !hasTextContent(node)
	}
	value, _ := node.Attr("value")
	return value == ""
}

func hasTextContent(node *uinode.Node) bool {
	if node == nil {
		return false
	}
	if node.Type == uinode.TextNode {
		return node.Data != ""
	}
	for _, child := range node.Children {
		if hasTextContent(child) {
			return true
		}
	}
	return false
}
