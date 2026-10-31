package dom

import (
	"strconv"
	"strings"
)

// StateSet stores explicitly resolved boolean states in two compact bitsets.
// A valid bit distinguishes a live false value from an uninitialized state.
type StateSet[T ~uint16] struct {
	values T
	valid  T
}

func (s StateSet[T]) Get(flag T) (bool, bool) {
	return s.values&flag != 0, s.valid&flag != 0
}

func (s *StateSet[T]) Set(flag T, value bool) {
	if s == nil {
		return
	}
	s.valid |= flag
	if value {
		s.values |= flag
	} else {
		s.values &^= flag
	}
}

func (s *StateSet[T]) Clear(flag T) {
	if s == nil {
		return
	}
	s.values &^= flag
	s.valid &^= flag
}

type ElementState uint16

const (
	StateChecked ElementState = 1 << iota
	StatePlaceholderShown
	StateIndeterminate
	StateDirtyChecked
	StateValueEmpty
	StateDirtyValue
	StateDirtySelected
)

func InitializeElementStates(root *Node) {
	var order uint32
	if initializeElementStates(root, &order) {
		initializeRadioStates(root, root)
	}
}

// AssignDocumentOrder records preorder positions without allocating.
func AssignDocumentOrder(root *Node) {
	var order uint32
	assignDocumentOrder(root, &order)
}

func assignDocumentOrder(node *Node, order *uint32) {
	if node == nil {
		return
	}
	node.Order = *order
	*order++
	for _, child := range node.Children {
		assignDocumentOrder(child, order)
	}
}

func initializeRadioStates(root, node *Node) {
	if node == nil {
		return
	}
	if node.Type == ElementNode && HasInputType(node, "radio") {
		name, _ := node.Attr("name")
		_, authoredChecked := node.Attr("checked")
		if authoredChecked && name != "" {
			clearCheckedRadio(root, node, name)
			node.States.Set(StateChecked, true)
		}
	}
	for _, child := range node.Children {
		initializeRadioStates(root, child)
	}
}

func clearCheckedRadio(node, except *Node, name string) {
	if node == nil {
		return
	}
	if node != except && node.Type == ElementNode && HasInputType(node, "radio") {
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

func ancestorTag(node *Node, tag string) *Node {
	for node = node.Parent; node != nil; node = node.Parent {
		if node.Type == ElementNode && node.Data == tag {
			return node
		}
	}
	return nil
}

// FormOwner returns the form associated with a form control. An explicit form
// attribute takes precedence over the nearest ancestor form.
func FormOwner(node *Node) *Node {
	if node == nil {
		return nil
	}
	if id, explicit := node.Attr("form"); explicit {
		return findFormByID(treeRoot(node), id)
	}
	return ancestorTag(node, "form")
}

func findFormByID(node *Node, id string) *Node {
	if node == nil || id == "" {
		return nil
	}
	if node.Type == ElementNode && node.Data == "form" {
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

func treeRoot(node *Node) *Node {
	for node != nil && node.Parent != nil {
		node = node.Parent
	}
	return node
}

func SameRadioGroup(left, right *Node) bool {
	if left == nil || right == nil || treeRoot(left) != treeRoot(right) || !HasInputType(left, "radio") || !HasInputType(right, "radio") {
		return false
	}
	leftName, _ := left.Attr("name")
	rightName, _ := right.Attr("name")
	return leftName != "" && leftName == rightName && FormOwner(left) == FormOwner(right)
}

// TabIndex parses the global tabindex attribute as a valid signed integer.
func TabIndex(node *Node) (int, bool) {
	if node == nil || node.Type != ElementNode {
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
func IsContentEditable(node *Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current.Type != ElementNode {
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
func IsEditingHost(node *Node) bool {
	if node == nil || node.Type != ElementNode || !IsContentEditable(node) {
		return false
	}
	parent := node.Parent
	for parent != nil && parent.Type != ElementNode {
		parent = parent.Parent
	}
	return !IsContentEditable(parent)
}

func initializeElementStates(node *Node, order *uint32) (hasRadio bool) {
	if node == nil {
		return false
	}
	node.Order = *order
	*order++
	if node.Type == ElementNode {
		switch node.Data {
		case "input":
			hasRadio = HasInputType(node, "radio")
			if hasRadio || HasInputType(node, "checkbox") {
				_, checked := node.Attr("checked")
				node.States.Set(StateChecked, checked)
			}
			value, _ := node.Attr("value")
			SetLiveValue(node, value, false)
		case "textarea":
			setLiveValueEmpty(node, !hasTextContent(node), false)
		case "option":
			_, selected := node.Attr("selected")
			node.States.Set(StateChecked, selected)
		}
	}
	for _, child := range node.Children {
		hasRadio = initializeElementStates(child, order) || hasRadio
	}
	if node.Type == ElementNode && node.Data == "select" {
		resetSelectStates(node)
	}
	return hasRadio
}

type selectResetState struct {
	index         int
	firstEnabled  int
	firstAuthored int
	lastAuthored  int
}

func resetSelectStates(node *Node) {
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

func resetSelectOptions(node *Node, state *selectResetState) {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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

func selectDisplaySize(node *Node) int {
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
func SelectedIndex(selectNode *Node) int {
	index, selected := 0, -1
	findSelectedIndex(selectNode, &index, &selected)
	return selected
}

func findSelectedIndex(node *Node, index, selected *int) bool {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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
func DefaultSelectedIndex(selectNode *Node) int {
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

func inspectSelectOptions(node *Node, state *selectResetState) {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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
func SetSelectedIndex(selectNode *Node, selected int) {
	var dirty *Node
	index := 0
	dirty = optionAt(selectNode, selected, &index)
	setSelectIndex(selectNode, selected, dirty)
}

// SetOptionSelected updates one option's selectedness as the selected IDL
// property does. A single-select keeps at most one selected option.
func SetOptionSelected(option *Node, selected bool) bool {
	if option == nil || option.Type != ElementNode || option.Data != "option" {
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

func SetOptionSelectedAt(selectNode *Node, optionIndex int, selected bool) bool {
	index := 0
	option := optionAt(selectNode, optionIndex, &index)
	return SetOptionSelected(option, selected)
}

// SetSelectedRange applies a user range selection to a multiple select in one
// mutation pass. Disabled options retain their selectedness because keyboard
// and pointer input cannot toggle them.
func SetSelectedRange(selectNode *Node, start, end int) bool {
	if selectNode == nil || selectNode.Type != ElementNode || selectNode.Data != "select" || start < 0 || end < 0 {
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

func selectOptionCount(node *Node) int {
	count := 0
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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

func setSelectedRange(node *Node, start, end int, index *int) {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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

func setSelectIndex(node *Node, selected int, dirty *Node) {
	index := 0
	setSelectOptions(node, selected, dirty, &index)
}

func setSelectOptions(node *Node, selected int, dirty *Node, index *int) {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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

func optionAt(node *Node, target int, index *int) *Node {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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
func SynchronizeSelectedAttribute(option *Node, present bool) {
	if option == nil || option.Type != ElementNode || option.Data != "option" {
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
func SynchronizeSelectAttribute(selectNode *Node, name string) {
	if selectNode == nil || selectNode.Type != ElementNode || selectNode.Data != "select" {
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
func SynchronizeSelectChildren(node *Node) {
	for node != nil && (node.Type != ElementNode || node.Data != "select") {
		node = node.Parent
	}
	if node == nil {
		return
	}
	if _, multiple := node.Attr("multiple"); multiple {
		return
	}
	var first, selected *Node
	normalizeSelectChildren(node, &first, &selected)
	if selected == nil && first != nil && selectDisplaySize(node) == 1 {
		first.States.Set(StateChecked, true)
	}
}

func normalizeSelectChildren(node *Node, first, selected **Node) {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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

func keepFirstSelectedOption(node *Node, selected *Node) *Node {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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

func unselectOtherOptions(node, selected *Node) {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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

func ensureSelectSelection(selectNode *Node) {
	if _, multiple := selectNode.Attr("multiple"); multiple || selectDisplaySize(selectNode) != 1 || SelectedIndex(selectNode) >= 0 {
		return
	}
	index, firstEnabled := 0, -1
	findFirstEnabledOption(selectNode, &index, &firstEnabled)
	setSelectIndex(selectNode, firstEnabled, nil)
}

func findFirstEnabledOption(node *Node, index, first *int) bool {
	for _, child := range node.Children {
		if child == nil || child.Type != ElementNode {
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

func HasDirtySelectedOption(selectNode *Node) bool {
	for _, child := range selectNode.Children {
		if child == nil || child.Type != ElementNode {
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
func ResetForm(form *Node) bool {
	if form == nil || form.Type != ElementNode || form.Data != "form" {
		return false
	}
	root := treeRoot(form)
	resetFormControls(root, form)
	normalizeResetRadioGroups(root, root, form)
	return true
}

func resetFormControls(node, form *Node) {
	if node == nil {
		return
	}
	if node.Type == ElementNode && FormOwner(node) == form {
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

func normalizeResetRadioGroups(root, node, form *Node) {
	if node == nil {
		return
	}
	if node.Type == ElementNode && HasInputType(node, "radio") && FormOwner(node) == form {
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

func optionDisabledForSelection(option *Node) bool {
	if option == nil || option.Type != ElementNode || option.Data != "option" {
		return true
	}
	if _, disabled := option.Attr("disabled"); disabled {
		return true
	}
	for ancestor := option.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type != ElementNode {
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

func IsChecked(node *Node) bool {
	if node == nil || node.Type != ElementNode {
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
func SynchronizeCheckedAttribute(node *Node, present bool) {
	if node == nil || node.Type != ElementNode || node.Data != "input" || !HasInputType(node, "checkbox") && !HasInputType(node, "radio") {
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

func SupportsDisabled(node *Node) bool {
	if node == nil || node.Type != ElementNode {
		return false
	}
	switch node.Data {
	case "button", "fieldset", "input", "optgroup", "option", "select", "textarea":
		return true
	default:
		return false
	}
}

func IsDisabled(node *Node) bool {
	if !SupportsDisabled(node) {
		return false
	}
	if _, disabled := node.Attr("disabled"); disabled {
		return true
	}
	if node.Data == "option" {
		for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Type == ElementNode && ancestor.Data == "optgroup" {
				if _, disabled := ancestor.Attr("disabled"); disabled {
					return true
				}
			}
			if ancestor.Type == ElementNode && ancestor.Data == "select" {
				return IsDisabled(ancestor)
			}
		}
	}
	for fieldset := node.Parent; fieldset != nil; fieldset = fieldset.Parent {
		if fieldset.Type != ElementNode || fieldset.Data != "fieldset" {
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

func withinFirstLegend(node, fieldset *Node) bool {
	var legend *Node
	for _, child := range fieldset.Children {
		if child != nil && child.Type == ElementNode && child.Data == "legend" {
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

func SupportsPlaceholder(node *Node) bool {
	if node == nil || node.Type != ElementNode {
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
func SetLiveValue(node *Node, value string, dirty bool) {
	setLiveValueEmpty(node, value == "", dirty)
}

func setLiveValueEmpty(node *Node, empty, dirty bool) {
	if node == nil || node.Type != ElementNode || node.Data != "input" && node.Data != "textarea" {
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
func SynchronizeValueAttribute(node *Node) {
	if node == nil || node.Type != ElementNode || node.Data != "input" {
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
func SynchronizeTextContent(node *Node) {
	if node == nil || node.Type != ElementNode || node.Data != "textarea" {
		return
	}
	dirty, _ := node.States.Get(StateDirtyValue)
	if dirty {
		return
	}
	setLiveValueEmpty(node, !hasTextContent(node), false)
}

func SynchronizePlaceholderAttribute(node *Node) {
	synchronizePlaceholderShown(node)
}

func synchronizePlaceholderShown(node *Node) {
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

func IsPlaceholderShown(node *Node) bool {
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

func hasTextContent(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Type == TextNode {
		return node.Data != ""
	}
	for _, child := range node.Children {
		if hasTextContent(child) {
			return true
		}
	}
	return false
}
