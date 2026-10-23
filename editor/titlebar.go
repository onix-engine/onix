package editor

import (
	"fmt"
	"strings"

	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	editoricons "github.com/diiyw/onix/editor/resources/images/icons"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

// Only action identifiers cross the Go/HTML boundary. Labels, icons, shortcuts
// and all presentation are authored in tilebar.html.
var titlebarMenuDefinitions = [...]struct {
	key     string
	actions []string
}{
	{"file", []string{"file.open-project", "file.open-recent-project", "file.new-project", "file.save", "file.reload", "file.close-project", "file.preferences"}},
	{"edit", []string{"edit.project-settings", "edit.undo", "edit.redo", "edit.toggle-grid-snap", "edit.select-all", "edit.delete"}},
	{"debug", []string{"debug.run", "debug.build", "debug.rebuild-preview", "debug.focus"}},
	{"about", []string{"about.show"}},
}

type titlebarMenu struct {
	key                  string
	trigger, popup, host htmlui.Node
}

type editorTitlebar struct {
	world          *ecs.World
	entity         ecs.Entity
	nodes          map[string]htmlui.Node
	menus          []titlebarMenu
	recentPaths    []string
	component      ecs.Entity
	hoverKnown     bool
	hoverX, hoverY float32
}

// Editor HTML assets are independent of the widget theme. Dialog previews and
// chrome icons resolve through stable DOM image sources; project images still
// fall through to the installed provider.
type editorHTMLImages struct {
	previous       htmlui.ImageProvider
	dialogTextures map[string]*graphics.Texture
}

func (p editorHTMLImages) ResolveHTMLImage(source string) (*graphics.Texture, bool) {
	if name, ok := strings.CutPrefix(source, "onix-file-icons/"); ok {
		texture := editorresources.FileIconTexture(name)
		return texture, texture != nil
	}
	if name, ok := strings.CutPrefix(source, "onix-dialog/"); ok {
		texture, found := p.dialogTextures[name]
		return texture, found && texture != nil
	}
	if name, ok := strings.CutPrefix(source, "onix-titlebar/"); ok {
		return editorHTMLIcon(name, graphics.ColorFromRGB(220, 224, 232)), true
	}
	if name, ok := strings.CutPrefix(source, "onix-leftbar/"); ok {
		return editorHTMLIcon(name, graphics.ColorFromRGB(176, 183, 195)), true
	}
	if name, ok := strings.CutPrefix(source, "onix-rightbar/"); ok {
		return editorHTMLIcon(name, graphics.ColorFromRGB(176, 183, 195)), true
	}
	if p.previous != nil {
		return p.previous.ResolveHTMLImage(source)
	}
	return nil, false
}

// Direction fragments reuse the SVG texture cache, including its rotated views.
func editorHTMLIcon(source string, color graphics.ColorRGBA) *graphics.Texture {
	name, direction, _ := strings.Cut(source, "#")
	turns := 0
	switch direction {
	case "up":
		turns = 2
	case "right":
		turns = 1
	case "danger":
		color = graphics.ColorFromRGB(196, 91, 104)
	}
	return editoricons.QuarterTurnsTexture(editoricons.Name(strings.TrimSuffix(name, ".svg")), 16, 16, color, turns)
}

func installEditorHTMLImages(world *ecs.World) {
	previous, _ := ecs.GetResource[htmlui.ImageProvider](world)
	if images, installed := previous.(editorHTMLImages); installed {
		if images.dialogTextures == nil {
			images.dialogTextures = make(map[string]*graphics.Texture)
			ecs.InsertResource[htmlui.ImageProvider](world, images)
		}
		return
	}
	ecs.InsertResource[htmlui.ImageProvider](world, editorHTMLImages{previous: previous, dialogTextures: make(map[string]*graphics.Texture)})
}

func registerEditorHTMLDialogTexture(world *ecs.World, source string, texture *graphics.Texture) {
	// GetResourceMut marks the provider changed; only do that when the source
	// actually resolves to a different texture so stable dialogs keep fast path.
	provider, ok := ecs.GetResource[htmlui.ImageProvider](world)
	if !ok {
		return
	}
	images, ok := provider.(editorHTMLImages)
	if !ok {
		return
	}
	if images.dialogTextures != nil && images.dialogTextures[source] == texture {
		return
	}
	if images.dialogTextures == nil {
		images.dialogTextures = make(map[string]*graphics.Texture)
	}
	images.dialogTextures[source] = texture
	if provider, ok := ecs.GetResourceMut[htmlui.ImageProvider](world); ok && provider != nil {
		*provider = images
	}
}

func (t *editorTitlebar) initView(world *ecs.World, font *onixtext.Font, rootBounds, top rect) error {
	*t = editorTitlebar{world: world, nodes: make(map[string]htmlui.Node)}
	html := htmlui.New(editorresources.TitlebarHTML)
	html.Font, html.FontSize = font, 14
	html.Width, html.Height = top.W, rootBounds.H
	bundle := ecs.NewBundle(7)
	ecs.Put(&bundle, scene.UITransformFromXY(top.X, top.Y))
	ecs.Put(&bundle, scene.UIGlobalTransform{})
	ecs.Put(&bundle, scene.Visibility{Visible: true})
	ecs.Put(&bundle, scene.AnchorTopLeft)
	ecs.Put(&bundle, scene.Size2D{})
	ecs.Put(&bundle, html)
	t.entity = world.Spawn(bundle)
	installEditorHTMLImages(world)
	root, err := htmlui.Root(world, t.entity)
	if err != nil {
		return err
	}
	bind := func(id string) (htmlui.Node, error) {
		node, ok := root.FindByID(id)
		if !ok {
			return htmlui.Node{}, fmt.Errorf("titlebar: missing node %q", id)
		}
		t.nodes[id] = node
		return node, nil
	}
	for _, id := range []string{"titlebar", "top-run", "top-save", "top-reload", "top-frame", "top-dimension", "run-icon", "window-controls", "window-minimize", "window-maximize", "window-close", "popup-recent", "recent-empty"} {
		if _, err := bind(id); err != nil {
			return err
		}
	}
	for _, def := range titlebarMenuDefinitions {
		trigger, err := bind("menu-" + def.key)
		if err != nil {
			return err
		}
		popup, err := bind("popup-" + def.key)
		if err != nil {
			return err
		}
		host, err := bind("menu-host-" + def.key)
		if err != nil {
			return err
		}
		t.menus = append(t.menus, titlebarMenu{def.key, trigger, popup, host})
		for _, item := range def.actions {
			if _, err := bind(item); err != nil {
				return err
			}
		}
	}
	definition, err := editorTitlebarDefinition()
	if err != nil {
		return err
	}
	t.component, err = htmlui.HydrateComponent(world, t.entity, definition, editorTitlebarView{}, titlebarComponentState{}, 1)
	if err != nil {
		return err
	}

	return nil
}

type editorTitlebarView struct {
	font                                                 *onixtext.Font
	root, top                                            rect
	menu, submenu                                        string
	recent                                               []string
	modal, customControls, project, dirty, running, snap bool
	dimension                                            previewDimension
}

func (t *editorTitlebar) syncView(world *ecs.World, view editorTitlebarView) error {
	if t.world != world || !world.IsAlive(t.entity) {
		if err := t.initView(world, view.font, view.root, view.top); err != nil {
			return err
		}
	}
	html := ecs.GetMut[htmlui.Surface](world, t.entity)
	html.Width, html.Height, html.Font = view.top.W, view.root.H, view.font
	ecs.GetMut[scene.UITransform](world, t.entity).Translation = scene.Px2(view.top.X, view.top.Y)
	modal := view.modal
	// A modal owns the whole window surface. Hide the titlebar entity so its
	// retained HTML visuals do not participate in the native UI draw pass.
	if visibility := ecs.GetMut[scene.Visibility](world, t.entity); visibility != nil {
		visibility.Visible = !modal
	}
	t.recentPaths = append(t.recentPaths[:0], view.recent[:min(len(view.recent), recentProjectLimit)]...)
	view.recent = t.recentPaths
	htmlui.UpdateComponentProps[editorTitlebarView, titlebarComponentState](world, t.component, view)
	return engineui.SyncHTML(world)
}
