package editor

import (
	"bytes"
	"maps"
	"slices"
	"strings"

	"github.com/onix-engine/onix/project"
)

// Cache compiled form data independently of document history and disk state.
// Invalid input keeps the last valid preview; unchanged forms allocate nothing.
type workspaceDraftPreview struct {
	source   *WorkspaceDocument
	drafts   map[string]string
	staged   map[DocumentID][]byte
	document *WorkspaceDocument
	renamed  *ModDocument
	revision uint64
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
		p.renamed = nil
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
	if a.previewNameDraft() {
		return
	}
	p.renamed = nil
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
		source := a.Document.Mods[mod.ID]
		if source == nil {
			return
		}
		modRevision := source.Revision
		if overrides[source.file] != nil {
			modRevision = revision
		}
		previous := source
		if p.document != nil {
			previous = p.document.Mods[mod.ID]
		}
		contentRevision := previous.contentRevision
		if previous.Revision != modRevision && !workspaceModContentEqual(previous.Spec, mod.Spec) {
			contentRevision = revision
		}
		document.Mods[mod.ID] = &ModDocument{ID: mod.ID, file: source.file, Spec: mod.Spec, Plan: mod, Revision: modRevision, contentRevision: contentRevision}
	}
	p.document = &document
}

// A module's display name has no compiled dependencies. Keep a private copy of
// its metadata across keystrokes so the inspector and plan agree on the name.
func (a *ModuleEditor) previewNameDraft() bool {
	if len(a.staged) != 0 || len(a.drafts) != 1 {
		return false
	}
	for id, name := range a.drafts {
		field := a.fields[id]
		if field.Kind != "string" || field.Pointer != "/name" || !field.modified(name) {
			return false
		}
		modID, ok := strings.CutSuffix(string(field.Target), "/"+project.ModFileName)
		mod := a.Document.Mods[project.ModID(modID)]
		if !ok || mod == nil || mod.Plan == nil {
			return false
		}
		p := &a.draftPreview
		if p.renamed == nil || p.renamed.ID != mod.ID {
			doc := *a.Document
			plan := *doc.Plan
			plan.Mods = slices.Clone(plan.Mods)
			index := slices.IndexFunc(plan.Mods, func(item project.ModPlan) bool { return item.ID == mod.ID })
			copyMod := *mod
			copyMod.Plan = &plan.Mods[index]
			doc.Plan, doc.Mods = &plan, maps.Clone(doc.Mods)
			doc.Mods[mod.ID] = &copyMod
			p.document, p.renamed = &doc, &copyMod
		}
		p.version++
		p.document.Revision = uint64(1)<<63 | p.version
		p.renamed.Revision = p.document.Revision
		p.renamed.Spec.Name, p.renamed.Plan.Spec.Name = name, name
	}
	return true
}
