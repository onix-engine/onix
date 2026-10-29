package editor

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics/codec/ontex"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/resource"
	htmlui "github.com/diiyw/onix/ui/html"
)

func moduleBuildFile(t *testing.T, directory, name, source string) {
	t.Helper()
	file := filepath.Join(directory, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func moduleBuildFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if _, err := CreateModuleProject(directory, project.CreateProjectOptions{Width: 640, Height: 480, ModulePath: "example.com/game"}); err != nil {
		t.Fatal(err)
	}
	moduleBuildFile(t, directory, "resources/project.json", `{"entry":"main.json","scenes":["main.json","game.json"],"types":{"Label":{"fields":{"text":"string","point":"vec2"}}}}`)
	moduleBuildFile(t, directory, "resources/scenes/main.json", `{"modules":[{"id":"a","mod":"a"},{"id":"b","mod":"b"}]}`)
	moduleBuildFile(t, directory, "resources/scenes/game.json", `{"modules":[{"id":"asset","mod":"asset"}]}`)
	moduleBuildFile(t, directory, "a/mod.json", `{"behavior":"Mod","slots":{"text":{"type":"string","default":"waiting"}}}`)
	moduleBuildFile(t, directory, "b/mod.json", `{"behavior":"Mod"}`)
	moduleBuildFile(t, directory, "asset/mod.json", `{"resources":{"private":{"kind":"data","path":"custom/value.dat"}}}`)
	moduleBuildFile(t, directory, "asset/custom/value.dat", "scene two asset")
	moduleBuildFile(t, directory, "a/mod.go", `package a
import (
  m "github.com/diiyw/onix/module"
  "github.com/diiyw/onix/project"
  "example.com/game/contracts"
)
type Mod struct { slot m.Slot }
func (s *Mod) Install(c *m.Context) error {
  var err error; s.slot,err=c.Slot("text"); if err!=nil { return err }
  p,err:=c.BindRequester[contracts.Label,string]("ask"); if err!=nil { return err }
  _,err=p.SendTo("b",contracts.Label{Text:"delivered",Point:[2]float32{1,2}},m.RequestOptions{}); return err
}
func (s *Mod) OnMessage(c *m.Context,message m.Message) error { v,ok:=message.ResultValue[string](); if !ok {return nil}
  if v.Err!=nil { return v.Err }; if err:=s.slot.Set(project.SlotValue{Type:project.StringValue,String:v.Value}); err!=nil { return err }; return c.Set(c.Root(),contracts.Label{Text:v.Value})
}
func (s *Mod) Update(*m.Context,m.Tick) error { return nil }
func (s *Mod) FrameUpdate(*m.Context,m.Tick) error { return nil }
func (s *Mod) Destroy(*m.Context) error { return nil }
`)
	moduleBuildFile(t, directory, "b/mod.go", `package b

import (
	"example.com/game/contracts"
	"github.com/diiyw/onix/module"
)

type Mod struct{}

func (*Mod) OnMessage(c *module.Context, message module.Message) error {
	v, ok := message.RequestValue[contracts.Label, string]()
	if !ok {
		return nil
	}
	return v.Reply(c, v.Value.Text)
}
`)
	return directory
}

func TestModuleBuildGeneratesTypedRegistryAndExecutesRequests(t *testing.T) {
	directory := moduleBuildFixture(t)
	mainBefore, _ := os.ReadFile(filepath.Join(directory, "main.go"))
	if _, err := GenerateModuleProject(directory); err != nil {
		t.Fatal(err)
	}
	mainAfter, _ := os.ReadFile(filepath.Join(directory, "main.go"))
	if !bytes.Equal(mainBefore, mainAfter) {
		t.Fatal("main.go was rewritten")
	}
	before, _ := os.ReadFile(filepath.Join(directory, "onix_generated.go"))
	if _, err := GenerateModuleProject(directory); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(directory, "onix_generated.go"))
	if !bytes.Equal(before, after) {
		t.Fatal("generation is not deterministic")
	}
	if strings.Count(string(after), "r.RegisterMod") != 2 || strings.Count(string(after), "OnMessage:") != 2 {
		t.Fatal("registry lacks independent duplicate type bindings")
	}
	if !strings.Contains(string(after), "FrameUpdate:") {
		t.Fatal("registry lacks the on-demand frame lifecycle")
	}
	moduleBuildFile(t, directory, "main_test.go", `package main
import (
 "os"
 "testing"
 "github.com/diiyw/onix/ecs"
 "github.com/diiyw/onix/module"
 "github.com/diiyw/onix/project"
 "example.com/game/contracts"
)
func TestGeneratedRuntime(t *testing.T) {
 plan,err:=project.CompileProjectDirectory("."); if err!=nil { t.Fatal(err) }
 reg,err:=OnixRegistry(); if err!=nil { t.Fatal(err) }
 world:=ecs.NewWorld(); runtime,err:=module.New[OnixContentProfile](world,plan,reg,module.Options{Source:os.DirFS(".")}); if err!=nil { t.Fatal(err) }; defer runtime.Close()
 _,err=runtime.MountEntry(); if err!=nil { t.Fatal(err) }
 for range 4 { if err=runtime.Dispatch(); err!=nil { t.Fatal(err) } }
 count:=0; for _,value:=range world.Query[contracts.Label]().All { count++; if value.Get().Text!="delivered" { t.Fatal("generated reply was not delivered") } }; if count!=1 { t.Fatal("missing private reply value") }
 value,err:=contracts.DecodeLabel(project.SlotValue{Type:"Label",Fields:[]project.SlotFieldValue{{Name:"point",Value:project.SlotValue{Type:project.Vec2Value,Vector:[4]float32{1,2}}},{Name:"text",Value:project.SlotValue{Type:project.StringValue,String:"typed"}}}})
 if err!=nil || value.Text!="typed" || value.Point!=[2]float32{1,2} { t.Fatal("typed decoder",value,err) }
 step:=func() { if err:=runtime.Update(0.016); err!=nil { panic(err) }; if err:=runtime.Dispatch(); err!=nil { panic(err) } }
 for range 8 { step() }; if a:=testing.AllocsPerRun(100,step); a!=0 { t.Fatal("generated warm lifecycle allocations",a) }
}
`)
	cmd := exec.Command("go", "test", "-mod=mod", ".", "./a", "./b", "./contracts")
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated independent project: %v\n%s", err, output)
	}
}

func TestModuleBuildDiagnosticsPreserveGeneratedAndUserSources(t *testing.T) {
	for _, tt := range []struct{ name, file, source, want string }{
		{"signature", "b/mod.go", `package b; import "github.com/diiyw/onix/module"; type Mod struct{}; func (*Mod) OnMessage(*module.Context,string) error { return nil }`, "signature"},
		{"dependency", "a/leak.go", `package a; import _ "example.com/game/b"`, "depends on module"},
		{"helper dependency", "tools/leak.go", `package tools; import _ "example.com/game/b"`, "depends on module"},
		{"protocol state", "contracts/state.go", `package contracts; func init(){}`, "data declarations only"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := moduleBuildFixture(t)
			if _, err := GenerateModuleProject(d); err != nil {
				t.Fatal(err)
			}
			baseline, _ := os.ReadFile(filepath.Join(d, "onix_generated.go"))
			moduleBuildFile(t, d, tt.file, tt.source)
			if _, err := GenerateModuleProject(d); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
			after, _ := os.ReadFile(filepath.Join(d, "onix_generated.go"))
			if !bytes.Equal(baseline, after) {
				t.Fatal("failed generation changed registry")
			}
		})
	}
}

func TestModuleBuildBinaryPackagesIncludeAllScenesAndReuseDefinitions(t *testing.T) {
	d := moduleBuildFixture(t)
	moduleBuildFile(t, d, "resources/packages.json", `{"packages":[{"name":"secondary","include":["asset/**"]}]}`)
	plan, err := GenerateModuleProject(d)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "publish")
	catalog, _, err := writeModuleResourcePackages(d, d, output, resourcePackageOptions{Mode: BuildModeRelease, Platform: BuildPlatformWeb})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Packages) != 2 {
		t.Fatal("missing module resource split")
	}
	manager, err := resource.OpenManager(output)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	decoded, err := project.ReadModulePlan(manager)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan, decoded) {
		t.Fatal("compiled indices, routes or defaults changed in binary")
	}
	asset, err := fs.ReadFile(manager, "asset/custom/value.dat")
	if err != nil || string(asset) != "scene two asset" {
		t.Fatal("secondary scene is missing its resource", err)
	}
	for _, p := range []string{"main.go", "resources/scenes/main.json", "a/mod.go", "a/mod.json", "resources/project.json"} {
		if _, err := fs.Stat(manager, p); !os.IsNotExist(err) {
			t.Fatalf("authoring file %s is published: %v", p, err)
		}
	}
	packageName := "secondary" + resource.Extension
	first, err := os.ReadFile(filepath.Join(output, packageName))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = writeModuleResourcePackages(d, d, output, resourcePackageOptions{Mode: BuildModeRelease, Platform: BuildPlatformWeb})
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(output, packageName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("cached package changed")
	}
	moduleBuildFile(t, d, "asset/custom/value.dat", "changed")
	_, _, err = writeModuleResourcePackages(d, d, output, resourcePackageOptions{Mode: BuildModeRelease, Platform: BuildPlatformWeb})
	if err != nil {
		t.Fatal(err)
	}
	third, err := os.ReadFile(filepath.Join(output, packageName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(second, third) {
		t.Fatal("private asset change did not invalidate the package cache")
	}
}

func TestModuleBuildPackagedReleaseLoadsBinaryAndPrivateAssets(t *testing.T) {
	d := moduleBuildFixture(t)
	moduleBuildFile(t, d, "main_test.go", `package main
import (
 "io/fs"
 "errors"
 "testing"
 "github.com/diiyw/onix/ecs"
 "github.com/diiyw/onix/module"
 "github.com/diiyw/onix/project"
 "example.com/game/contracts"
)
func TestReleasePackage(t *testing.T) {
 plan,source,err:=project.OpenModuleProject(); if err!=nil { t.Fatal(err) }
 if len(plan.Scenes)!=2 { t.Fatal("missing scene plan") }
 if _,err=fs.Stat(source,"resources/scenes/main.json"); !errors.Is(err,fs.ErrNotExist) { t.Fatal("raw scene JSON is mounted",err) }
 asset,err:=fs.ReadFile(source,"asset/custom/value.dat"); if err!=nil || string(asset)!="scene two asset" { t.Fatal("missing asset",err) }
 reg,err:=OnixRegistry(); if err!=nil { t.Fatal(err) }
 world:=ecs.NewWorld(); r,err:=module.New[OnixContentProfile](world,plan,reg,module.Options{Source:source}); if err!=nil { t.Fatal(err) }; defer r.Close()
 if _,err=r.MountEntry(); err!=nil { t.Fatal(err) }; for range 4 { if err=r.Dispatch(); err!=nil { t.Fatal(err) } }
 count:=0; for _,v:=range world.Query[contracts.Label]().All { count++; if v.Get().Text!="delivered" { t.Fatal("missing reply") } }; if count!=1 { t.Fatal("missing private component") }
}
`)
	build, err := PrepareDevelopmentBuild(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer build.Cleanup()
	cmd := exec.Command("go", "test", "-mod=mod", "-tags", "onix_release", "-overlay", build.OverlayPath, ".")
	cmd.Dir = d
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "ONIX_RESOURCE_DIR="+build.ResourceDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("headless release package failed: %v\n%s", err, output)
	}

	binary := filepath.Join(t.TempDir(), "game")
	compile := exec.Command("go", "build", "-p", "2", "-mod=mod", "-buildvcs=false", "-trimpath", "-tags", "onix_release", "-overlay", build.OverlayPath, "-o", binary, ".")
	compile.Dir, compile.Env = d, cmd.Env
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("build release for link inspection: %v\n%s", err, output)
	}
	symbols, err := exec.Command("go", "tool", "nm", binary).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect release symbols: %v\n%s", err, symbols)
	}
	for _, forbidden := range []string{
		"github.com/diiyw/onix/project.(*Resources).ModContent",
		"github.com/diiyw/onix/ui/html.renderHTML",
		"github.com/diiyw/onix/html/style.applyProperty",
		"github.com/diiyw/onix/spine.restoreResource",
		"github.com/diiyw/onix/physics.(*World).buildBroadphase",
		"github.com/diiyw/onix/scene/render3d.UpdateDeformationsSystem",
	} {
		if bytes.Contains(symbols, []byte(forbidden)) {
			t.Errorf("2D release retains optional implementation %s", forbidden)
		}
	}
}

func TestModuleBuildChecksEmbeddedAssetsAndSelectsOnlyRequiredFeatures(t *testing.T) {
	d := moduleBuildFixture(t)
	moduleBuildFile(t, d, "resources/project.json", `{"entry":"game.json","scenes":["game.json"],"engine":{"features":["render2d","animations2d"],"imageDecoders":["png"]}}`)
	if err := os.Remove(filepath.Join(d, "resources", "scenes", "main.json")); err != nil {
		t.Fatal(err)
	}
	// Unused behaviors are still checked: remove them to isolate this pure JSON project.
	if err := os.Remove(filepath.Join(d, "a", "mod.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(d, "b", "mod.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(d, "a", "mod.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(d, "b", "mod.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateModuleProject(d); err != nil {
		t.Fatal(err)
	}
	moduleBuildFile(t, d, "unused/mod.json", `{"behavior":"Mod"}`)
	moduleBuildFile(t, d, "unused/mod.go", `package unused; type Mod struct{}`)
	if _, err := GenerateModuleProject(d); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(d, "onix_generated.go"))
	source := string(data)
	if !strings.Contains(source, "type OnixContentProfile = module.WithContent[module.Render2D, module.AnimationsContent]") || !strings.Contains(source, "InstallSystems[onix.Animations2D]") || !strings.Contains(source, "graphics/codec/png") || strings.Contains(source, "graphics/codec/jpeg") || strings.Contains(source, "InstallSystems[onix.Render3D]") || strings.Contains(source, "example.com/game/unused") {
		t.Fatal("generation did not trim unused features and decoders")
	}
	moduleBuildFile(t, d, "private/mod.json", `{}`)
	moduleBuildFile(t, d, "asset/embed.go", `package asset
import _ "embed"
//go:embed ../private/mod.json
var data []byte
`)
	if _, err := CheckModuleProject(d); err == nil || !strings.Contains(err.Error(), "must stay within its module") {
		t.Fatal("embed escape accepted", err)
	}
}

func TestModuleBuildSelectsContentLifecycle(t *testing.T) {
	d := moduleBuildFixture(t)
	for _, test := range []struct {
		name, component, features, profile string
	}{
		{"sprite", `"sprite":{"color":"#ffffff"}`, `[]`, "Render2D"},
		{"layout", `"layout":{}`, `[]`, "WithContent[module.Render2D, module.LayoutContent]"},
		{"collider", `"rigidBody2D":{},"circleCollider2D":{"radius":10}`, `[]`, "WithContent[module.Render2D, module.Physics2DContent]"},
		{"script HTML", `"sprite":{"color":"#ffffff"}`, `["render2d","input2d","ui-widgets","ui-html"]`, "WithContent[module.WithContent[module.Render2D, module.LayoutContent], module.HTMLContent]"},
		{"animation", `"sprite":{"color":"#ffffff"}`, `["render2d","animations2d"]`, "WithContent[module.Render2D, module.AnimationsContent]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			moduleBuildFile(t, d, "resources/project.json", `{"entry":"game.json","scenes":["main.json","game.json"],"engine":{"features":`+test.features+`},"types":{"Label":{"fields":{"text":"string","point":"vec2"}}}}`)
			moduleBuildFile(t, d, "asset/mod.json", `{"space":"world","components":{`+test.component+`}}`)
			// Unreferenced content must not force the full profile.
			moduleBuildFile(t, d, "unused/mod.json", `{"components":{"layout":{}}}`)
			if _, err := GenerateModuleProject(d); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(d, "onix_generated.go"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "type OnixContentProfile = module."+test.profile) {
				t.Fatalf("wrong generated profile, want %s", test.profile)
			}
		})
	}
}

func TestModuleBuildJavaDiagnosticAndEditorBuildState(t *testing.T) {
	home := t.TempDir()
	moduleBuildFile(t, home, "bin/"+androidExecutableName("java"), "")
	moduleBuildFile(t, home, "bin/"+androidExecutableName("javac"), "")
	moduleBuildFile(t, home, "release", `JAVA_VERSION="26.0.1"`)
	if _, err := validateAndroidJavaHome(home, "JAVA_HOME"); err == nil || !strings.Contains(err.Error(), "Gradle 8.9 requires Java 17 through 22") {
		t.Fatal("unsupported Gradle JVM was accepted", err)
	}
	a := workspaceEditorFixture(t)
	a.action("build")
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	_, hasTitle := htmlui.NodeByID(world, a.dialogUI.htmlDialogs["build"], "build-title")
	if !a.buildOpen || !hasTitle {
		t.Fatal("build dialog is missing")
	}
	workspaceDialogTestEvent(t, a, world, "build-close", "", htmlui.EventClick)
	if a.buildOpen {
		t.Fatal("build dialog did not close")
	}
	a.buildDone = make(chan buildResult, 1)
	a.buildProgress = newBuildProgressFeed(nil)
	if a.closeRequested() {
		t.Fatal("editor closed while build owns project files")
	}
	a.buildDone <- buildResult{artifact: "output.zip"}
	a.updateModuleBuild()
	if a.buildDone != nil || !strings.Contains(a.status, "output.zip") {
		t.Fatal("editor did not consume build result")
	}
}

func TestModuleBuildRejectsPrivateAssetDependencyEscape(t *testing.T) {
	for _, tt := range []struct{ name, ref, want string }{{"escape", "../../a/private.png", "escapes module"}, {"missing", "missing.png", "missing resource dependency"}} {
		t.Run(tt.name, func(t *testing.T) {
			d := moduleBuildFixture(t)
			moduleBuildFile(t, d, "asset/mod.json", `{"resources":{"atlas":{"kind":"atlas","path":"custom/pack.atlas"}}}`)
			encoded, _ := json.Marshal(map[string]any{"meta": map[string]string{"image": tt.ref}})
			moduleBuildFile(t, d, "asset/custom/pack.atlas", string(encoded))
			if _, err := CheckModuleProject(d); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q: %v", tt.want, err)
			}
		})
	}
}

func TestModuleBuildAtlasFrameNamesAreNotFileDependencies(t *testing.T) {
	d := moduleBuildFixture(t)
	moduleBuildFile(t, d, "asset/mod.json", `{"components":{"sprite":{"texture":{"atlas":"custom/pack.json","frame":"bird.png"}}}}`)
	moduleBuildFile(t, d, "asset/custom/pack.json", `{"meta":{"image":"sheet.png"},"frames":{"bird.png":{"frame":{"x":0,"y":0,"w":1,"h":1},"filename":"bird.png"}}}`)
	moduleBuildFile(t, d, "asset/custom/sheet.png", "packed image")
	if _, err := CheckModuleProject(d); err != nil {
		t.Fatal("atlas frame identity treated as a source file", err)
	}
}

func TestModuleBuildGeneratedFileOwnershipAndReadOnlyCheck(t *testing.T) {
	d := moduleBuildFixture(t)
	before, _ := os.ReadFile(filepath.Join(d, "onix_generated.go"))
	if _, err := CheckModuleProject(d); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(d, "onix_generated.go"))
	if !bytes.Equal(before, after) {
		t.Fatal("check wrote generated registry")
	}
	if _, err := os.Stat(filepath.Join(d, "contracts", "onix_generated.go")); !os.IsNotExist(err) {
		t.Fatal("check created contracts")
	}
	moduleBuildFile(t, d, "onix_generated.go", "package main\n// user-owned\n")
	if _, err := GenerateModuleProject(d); err == nil || !strings.Contains(err.Error(), "user file was preserved") {
		t.Fatal(err)
	}
}

func TestModuleBuildAndroidStagingSupportsIndependentProject(t *testing.T) {
	d := moduleBuildFixture(t)
	if _, err := GenerateModuleProject(d); err != nil {
		t.Fatal(err)
	}
	layout, err := stageAndroidProject(d, d, BuildModeRelease, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.AppDir, "go.mod")); !os.IsNotExist(err) {
		t.Fatal("nested Go module was copied into mobile stage")
	}
	registry, err := os.ReadFile(filepath.Join(layout.AppDir, "onix_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(registry), "package "+layout.PackageName) || !strings.Contains(string(registry), "example.com/game/a") {
		t.Fatal("staged registry lost independent import paths")
	}
	source, err := os.ReadFile(filepath.Join(layout.AppDir, "zz_generated_onix_resources_mobile.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `"github.com/diiyw/onix/resource"`) || !strings.Contains(string(source), `SetDefaultProject(source, ".")`) {
		t.Fatal("mobile package source uses the project's module as engine import")
	}
}

func TestModulePackagesPreserveUltraHDRJPEGWithTextureCompression(t *testing.T) {
	d := moduleBuildFixture(t)
	moduleBuildFile(t, d, "resources/project.json", `{"entry":"main.json","scenes":["main.json"],"engine":{"textureCompression":true}}`)
	moduleBuildFile(t, d, "resources/scenes/main.json", `{"modules":[{"id":"a","mod":"a"}]}`)
	moduleBuildFile(t, d, "a/mod.json", `{"space":"world","components":{"camera":{"environment":{"path":"resources/sky.jpg","intensity":1,"background":true}}},"elements":[{"id":"other-camera","components":{"camera":{"environment":{"path":"resources/other.jpeg","intensity":1}}}}]}`)
	original, err := os.ReadFile("../testdata/resources/environment/royal_esplanade_2k.hdr.jpg")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sky.jpg", "other.jpeg"} {
		file := filepath.Join(d, "a/resources", name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, original, 0644); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(t.TempDir(), "publish")
	if _, _, err := writeModuleResourcePackages(d, d, output, resourcePackageOptions{Mode: BuildModeRelease, Platform: BuildPlatformLinux}); err != nil {
		t.Fatal(err)
	}
	manager, err := resource.OpenManager(output)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	for _, name := range []string{"sky.jpg", "other.jpeg"} {
		packed, err := fs.ReadFile(manager, "a/resources/"+name)
		if err != nil || !bytes.Equal(packed, original) {
			t.Fatalf("Ultra HDR %s was rewritten: %v", name, err)
		}
	}
}

func TestModulePackagesPreserveThemedSVGWithTextureCompression(t *testing.T) {
	directory := moduleBuildFixture(t)
	moduleBuildFile(t, directory, "resources/project.json", `{"entry":"main.json","scenes":["main.json"],"engine":{"textureCompression":true}}`)
	moduleBuildFile(t, directory, "resources/scenes/main.json", `{"modules":[{"id":"a","mod":"a"}]}`)
	moduleBuildFile(t, directory, "a/mod.json", `{"resources":{"themed":{"kind":"data","path":"resources/themed.svg"},"static":{"kind":"data","path":"resources/static.svg"}}}`)
	themed := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M2 2h12v12H2z"/></svg>`)
	moduleBuildFile(t, directory, "a/resources/themed.svg", string(themed))
	moduleBuildFile(t, directory, "a/resources/static.svg", strings.ReplaceAll(string(themed), "currentColor", "#ffffff"))
	output := filepath.Join(t.TempDir(), "publish")
	if _, _, err := writeModuleResourcePackages(directory, directory, output, resourcePackageOptions{Mode: BuildModeRelease, Platform: BuildPlatformLinux}); err != nil {
		t.Fatal(err)
	}
	manager, err := resource.OpenManager(output)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	packed, err := fs.ReadFile(manager, "a/resources/themed.svg")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packed, themed) {
		t.Fatal("packaging rasterized the theme-dependent SVG; runtime foreground can no longer replace currentColor")
	}
	packed, err = fs.ReadFile(manager, "a/resources/static.svg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ontex.Unmarshal(packed); err != nil {
		t.Fatalf("static SVG bypassed texture compression: %v", err)
	}
}
