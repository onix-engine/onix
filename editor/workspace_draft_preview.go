package editor

import (
	"bytes"
	"maps"

	"github.com/diiyw/onix/project"
)

// Cache compiled form data independently of document history and disk state.
// Invalid input keeps the last valid preview; unchanged forms allocate nothing.
type workspaceDraftPreview struct {
	source   *WorkspaceDocument
	revision uint64
	drafts   map[string]string
	staged   map[DocumentID][]byte
	document *WorkspaceDocument
	version  uint64
}

func (a *ModuleEditor) previewDocument() *WorkspaceDocument {
	p := &a.draftPreview
	if p.source == a.Document && p.revision == a.Document.Revision && p.document != nil {
		p.document.Active = a.Document.Active
		return p.document
	}
	return a.Document
}

func (a *ModuleEditor) syncDraftPreview() {
	p := &a.draftPreview
	sourceChanged := p.source != a.Document || p.revision != a.Document.Revision
	if !sourceChanged && maps.Equal(p.drafts, a.drafts) && maps.EqualFunc(p.staged, a.staged, bytes.Equal) {
		return
	}
	if sourceChanged {
		p.document = nil
	}
	p.source, p.revision = a.Document, a.Document.Revision
	if p.drafts == nil {
		p.drafts = make(map[string]string)
		p.staged = make(map[DocumentID][]byte)
	}
	clear(p.drafts)
	clear(p.staged)
	maps.Copy(p.drafts, a.drafts)
	maps.Copy(p.staged, a.staged)
	if !a.hasDrafts() {
		p.document = nil
		return
	}
	patches, err := a.formPatches()
	if err != nil {
		return
	}
	overrides := make(map[DocumentID][]byte, len(patches))
	for target, values := range patches {
		state := a.Document.states[target]
		if state == nil {
			return
		}
		data, err := applyDocumentPatches(state.data, values)
		if err != nil {
			return
		}
		if !bytes.Equal(data, state.data) {
			overrides[target] = data
		}
	}
	plan, err := a.Document.compile(overrides)
	if err != nil {
		return
	}
	p.version++
	// Draft revisions cannot collide with committed document revisions.
	revision := uint64(1)<<63 | p.version
	document := *a.Document
	document.Plan, document.Spec, document.Revision = plan, plan.Spec, revision
	document.Scenes = make(map[project.SceneID]*SceneDocument, len(plan.Scenes))
	document.Mods = make(map[project.ModID]*ModDocument, len(plan.Mods))
	for i := range plan.Scenes {
		scene := &plan.Scenes[i]
		document.Scenes[scene.ID] = &SceneDocument{ID: scene.ID, Spec: scene.Spec, Plan: scene, Revision: revision}
	}
	for i := range plan.Mods {
		mod := &plan.Mods[i]
		modRevision := a.Document.Mods[mod.ID].Revision
		if overrides[modDocumentID(mod.ID)] != nil {
			modRevision = revision
		}
		previous := a.Document.Mods[mod.ID]
		if p.document != nil {
			previous = p.document.Mods[mod.ID]
		}
		contentRevision := previous.contentRevision
		if previous.Revision != modRevision && !workspaceModContentEqual(previous.Spec, mod.Spec) {
			contentRevision = revision
		}
		document.Mods[mod.ID] = &ModDocument{ID: mod.ID, Spec: mod.Spec, Plan: mod, Revision: modRevision, contentRevision: contentRevision}
	}
	p.document = &document
}
