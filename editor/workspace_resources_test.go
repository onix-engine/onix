package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
)

type workspaceShaderTestDriver struct {
	*viewport3DTestDriver
	created, draws         int
	meshCreated, meshDraws int
	meshUniform            float32
}

func (d *workspaceShaderTestDriver) CreateMeshShader(descriptor render.ShaderDescriptor) (render.MeshShaderHandle, error) {
	if err := descriptor.Validate(); err != nil {
		return render.MeshShaderHandle{}, err
	}
	d.meshCreated++
	return render.NewHandle[render.MeshShaderResource](uint32(d.meshCreated), 1), nil
}

func (d *workspaceShaderTestDriver) DestroyMeshShader(render.MeshShaderHandle) error { return nil }

func (d *workspaceShaderTestDriver) DrawMeshes(batch render.MeshBatch3D) error {
	for _, draw := range batch.Draws {
		if draw.Shader.Valid() {
			d.meshDraws += int(draw.InstanceCount)
			d.meshUniform = batch.Instances[draw.FirstInstance].Custom[0][0]
		}
	}
	return d.viewport3DTestDriver.DrawMeshes(batch)
}

func TestWorkspaceShader3DPreviewUsesElementShaderComponent(t *testing.T) {
	driver := &workspaceShaderTestDriver{viewport3DTestDriver: &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 960}}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 960)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a := workspaceEditorFixture(t)
	document, err := LoadWorkspaceDocument(filepath.Join("..", "projects", "shader3d"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Document.Dispose(); err != nil {
		t.Fatal(err)
	}
	a.Document, a.dimension = document, previewDimension3D
	if err := document.EditMod("effect", "Shader uniform", func(mod *project.ModSpec) error {
		mod.Elements[2].Components.Shader.Custom[0][0] = .75
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		driver.meshDraws = 0
		if err := a.render(context); err != nil {
			t.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		if driver.meshDraws != 1 || driver.meshCreated != 1 || driver.meshUniform != .75 {
			t.Fatalf("mesh shader draws=%d, programs=%d, uniform=%g", driver.meshDraws, driver.meshCreated, driver.meshUniform)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := a.syncViewport3D(); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("unchanged mesh shader preview allocated %g", allocations)
	}
	if document.Mods["effect"].Plan.Elements[2].Spec.Components.Shader.WGSLFragment == "" {
		t.Fatal("preview replaced the authored shader source")
	}
}

func (d *workspaceShaderTestDriver) CreateMaterial2DShader(descriptor render.ShaderDescriptor) (render.Material2DShaderHandle, error) {
	if err := descriptor.Validate(); err != nil {
		return render.Material2DShaderHandle{}, err
	}
	d.created++
	return render.NewHandle[render.Material2DShaderResource](uint32(d.created), 1), nil
}

func (d *workspaceShaderTestDriver) DestroyMaterial2DShader(render.Material2DShaderHandle) error {
	return nil
}

func (d *workspaceShaderTestDriver) DrawSprites(batch render.SpriteBatch) error {
	for _, draw := range batch.Draws {
		if draw.Shader.Valid() {
			d.draws += int(draw.InstanceCount)
		}
	}
	return nil
}

func TestWorkspaceShader2DPreviewCompilesAndDrawsPrivateContent(t *testing.T) {
	driver := &workspaceShaderTestDriver{viewport3DTestDriver: &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 960}}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 960)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a := workspaceEditorFixture(t)
	document, err := LoadWorkspaceDocument(filepath.Join("..", "projects", "shader2d"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Document.Dispose(); err != nil {
		t.Fatal(err)
	}
	a.Document = document
	sourcePath := document.Absolute("effect/mod.json")
	before, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	for _, local := range []project.ModID{"", "effect", ""} {
		a.Local = local
		a.changed()
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		if a.viewport2D.err != nil {
			t.Fatal(a.viewport2D.err)
		}
		driver.draws = 0
		if err := a.renderViewport2D(canvas); err != nil {
			t.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		if driver.draws != 1 || driver.created != 1 {
			t.Fatalf("preview %q: shader draws=%d, compilations=%d", local, driver.draws, driver.created)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := a.syncViewport2D(); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("unchanged shader preview allocated %g", allocations)
	}
	if err := document.EditMod("effect", "Change shader uniforms", func(mod *project.ModSpec) error {
		mod.Elements[0].Components.Shader.Custom[0][0] = 2
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	row := &a.viewport2D.rows[1]
	shaderComponent := ecs.GetRef[scene.Shader[graphics.Material2DShader]](a.viewport2D.world, row.parts[0])
	if driver.created != 2 || shaderComponent.Custom[0][0] != 2 {
		t.Fatal("content edits did not refresh the shader preview")
	}
	shader := document.Mods["effect"].Plan.Elements[0].Spec.Components.Shader
	if shader.WGSLFragment == "" || shader.Compiled.HasSource() {
		t.Fatal("preview replaced the authoring shader with compiled paths")
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil || string(after) != string(before) {
		t.Fatal("preview modified the source manifest", err)
	}
}
