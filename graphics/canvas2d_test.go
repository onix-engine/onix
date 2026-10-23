package graphics

import (
	"errors"
	"image"
	"math"
	"math/rand"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func TestMultiplyAffineMatchesMatrixComposition(t *testing.T) {
	leftMatrix := mgl32.Translate3D(12, -7, 0).Mul4(mgl32.HomogRotate3DZ(0.35)).Mul4(mgl32.Scale3D(1.5, 0.75, 1))
	rightMatrix := mgl32.Translate3D(-3, 9, 0).Mul4(mgl32.HomogRotate3DZ(-0.2)).Mul4(mgl32.Scale3D(4, 2, 1))
	left := affineFromMat4(leftMatrix)
	right := affineFromMat4(rightMatrix)
	composed := leftMatrix.Mul4(rightMatrix)
	want := affineFromMat4(composed)

	got := multiplyAffine(left, right)
	for index := range got {
		if math.Abs(float64(got[index]-want[index])) > 1e-5 {
			t.Fatalf("affine[%d] = %f, want %f", index, got[index], want[index])
		}
	}
}

func TestCanvasRasterScaleDefaultsAndNormalizes(t *testing.T) {
	canvas := Canvas2DContext(321, 181)
	canvas.SetRasterScale(1)
	if got := canvas.RasterScale(); got != 1 {
		t.Fatalf("default raster scale = %v, want 1", got)
	}
	canvas.SetRasterScale(2)
	if got := canvas.RasterScale(); got != 2 {
		t.Fatalf("raster scale = %v, want 2", got)
	}
	canvas.SetRasterScale(0)
	if got := canvas.RasterScale(); got != 1 {
		t.Fatalf("normalized raster scale = %v, want 1", got)
	}
}

func TestDrawAffineMatchesMatrixEntry(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(),
		width: 320, height: 240,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
		instances:  make([]render.Instance2D, 0, 2),
	}
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	view := mgl32.Translate3D(17, -11, 0).Mul4(mgl32.HomogRotate3DZ(0.31)).Mul4(mgl32.Scale3D(1.25, 0.8, 1))
	model := mgl32.Translate3D(-5, 13, 0).Mul4(mgl32.HomogRotate3DZ(-0.22)).Mul4(mgl32.Scale3D(3, 2, 1))
	textureModel := mgl32.Translate2D(0.125, 0.25).Mul3(mgl32.Scale2D(0.5, 0.75))
	tint := ColorRGBA{R: 0.25, G: 0.5, B: 0.75, A: 0.8}

	context.SetViewMatrix(view)
	context.BindTexture(texture)
	context.DrawTintedMatrices(model, textureModel, tint, 0.6)
	context.DrawTintedAffine(affineFromMat4(model), affineFromMat3(textureModel), tint, 0.6)

	if len(context.instances) != 2 {
		t.Fatalf("sprite instances = %d, want 2", len(context.instances))
	}
	if err := context.FinishFrame(); err != nil {
		t.Fatalf("finish frame: %v", err)
	}
	if len(driver.spriteBatches) != 1 || len(driver.spriteBatches[0].Instances) != 2 {
		t.Fatalf("sprite batches = %+v, want one batch with two instances", driver.spriteBatches)
	}
	instances := driver.spriteBatches[0].Instances
	if instances[0] != instances[1] {
		t.Fatalf("affine entry = %+v, matrix entry = %+v", instances[1], instances[0])
	}
	want := affineFromMat4(view.Mul4(model))
	for index, value := range instances[0].Model {
		if math.Abs(float64(value-want[index])) > 1e-5 {
			t.Fatalf("model[%d] = %f, want %f", index, value, want[index])
		}
	}
}

func TestApplyViewAffineBatchMatchesGo(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	instances := make([]render.Instance2D, 257)
	for index := range instances {
		for valueIndex := range instances[index].Model {
			instances[index].Model[valueIndex] = random.Float32()*200 - 100
		}
		for valueIndex := range instances[index].TextureModel {
			instances[index].TextureModel[valueIndex] = random.Float32()*200 - 100
		}
		for valueIndex := range instances[index].Color {
			instances[index].Color[valueIndex] = random.Float32()*200 - 100
		}
		for row := range instances[index].Custom {
			for column := range instances[index].Custom[row] {
				instances[index].Custom[row][column] = random.Float32()*200 - 100
			}
		}
	}
	view := [6]float32{}
	for index := range view {
		view[index] = random.Float32()*20 - 10
	}
	want := append([]render.Instance2D(nil), instances...)
	applyViewAffineBatchGo(want, view)
	applyViewAffineBatch(instances, view)

	for index := range instances {
		if instances[index] != want[index] {
			t.Fatalf("instance %d = %+v, want %+v", index, instances[index], want[index])
		}
	}
}

func TestClipRectMapsLogicalCoordinatesIntoRenderViewport(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{X: 100, Y: 50, Width: 800, Height: 600}}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(), width: 400, height: 300}

	context.PushClipRect(10.25, 20.25, 30.25, 40.25)

	want := render.Rect{X: 120, Y: 90, Width: 61, Height: 81}
	if driver.scissor == nil || *driver.scissor != want {
		t.Fatalf("unexpected physical scissor: got %+v want %+v", driver.scissor, want)
	}
}

func TestFinishFrameReturnsAndClearsDriverError(t *testing.T) {
	want := errors.New("begin failed")
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}, beginErr: want}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(), width: 320, height: 240}

	context.ClearColor(ColorRGBA{})
	if got := context.FinishFrame(); !errors.Is(got, want) {
		t.Fatalf("unexpected frame error: got %v want %v", got, want)
	}
	if got := context.FinishFrame(); got != nil {
		t.Fatalf("expected frame error to be cleared, got %v", got)
	}
}

func TestTextureChangeSharesSpriteDraw(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(),
		width:      320,
		height:     240,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
		instances:  make([]render.Instance2D, 0, 2),
		draws:      make([]render.SpriteDraw, 0, 2),
	}
	first := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	second := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))

	context.BindTexture(first)
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	context.BindTexture(second)
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	if err := context.FinishFrame(); err != nil {
		t.Fatalf("finish frame: %v", err)
	}

	if len(driver.spriteBatches) != 1 || len(driver.spriteBatches[0].Draws) != 1 {
		t.Fatalf("unexpected sprite batches: %+v", driver.spriteBatches)
	}
	draw := driver.spriteBatches[0].Draws[0]
	if draw.TextureAt(0) != first.RenderHandle() || draw.TextureAt(1) != second.RenderHandle() || draw.InstanceCount != 2 {
		t.Fatalf("unexpected texture slots: %+v", draw)
	}
}

func TestDifferentTintsShareOneSpriteDraw(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(),
		width: 320, height: 240,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
	}
	texture := CreateTextureByColor(White)
	red := ColorFromRGBA(255, 0, 0, 255)
	blue := ColorFromRGBA(0, 0, 255, 255)

	context.BindTexture(texture)
	context.DrawTintedMatrices(mgl32.Ident4(), mgl32.Ident3(), red, 1)
	context.DrawTintedMatrices(mgl32.Ident4(), mgl32.Ident3(), blue, 1)
	if err := context.FinishFrame(); err != nil {
		t.Fatal(err)
	}

	if len(driver.spriteBatches) != 1 || len(driver.spriteBatches[0].Draws) != 1 {
		t.Fatalf("tinted sprite draws = %+v, want one draw", driver.spriteBatches)
	}
	batch := driver.spriteBatches[0]
	if batch.Draws[0].InstanceCount != 2 || batch.Instances[0].Color != [4]float32{1, 0, 0, 1} || batch.Instances[1].Color != [4]float32{0, 0, 1, 1} {
		t.Fatalf("tinted sprite batch = %+v", batch)
	}
}

func TestProjectionChangeFlushesSpriteBatch(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(),
		width:      320,
		height:     240,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
	}
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	worldProjection := mgl32.Ortho(-160, 160, -120, 120, -1000, 1000)

	context.BindTexture(texture)
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	context.SetProjectionMatrix(worldProjection)
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	if err := context.FinishFrame(); err != nil {
		t.Fatalf("finish frame: %v", err)
	}

	if driver.submits != 2 || len(driver.projections) != 2 {
		t.Fatalf("projection change submitted %d batches with %d projections, want 2", driver.submits, len(driver.projections))
	}
	if driver.projections[0] != [16]float32(mgl32.Ortho2D(0, 320, 240, 0)) {
		t.Fatalf("first projection is not the UI projection: %v", driver.projections[0])
	}
	if driver.projections[1] != [16]float32(worldProjection) {
		t.Fatalf("second projection is not the world projection: %v", driver.projections[1])
	}
}

func TestViewChangeFlushesSpriteBatch(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(),
		width:      320,
		height:     240,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
	}
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	translated := mgl32.Translate3D(40, -25, 0)

	context.BindTexture(texture)
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	context.SetViewMatrix(translated)
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	if err := context.FinishFrame(); err != nil {
		t.Fatalf("finish frame: %v", err)
	}

	if len(driver.spriteBatches) != 2 {
		t.Fatalf("view change submitted %d sprite batches, want 2", len(driver.spriteBatches))
	}
	if got := driver.spriteBatches[0].Instances[0].Model; got != affineFromMat4(mgl32.Ident4()) {
		t.Fatalf("first batch model = %v, want identity", got)
	}
	if got := driver.spriteBatches[1].Instances[0].Model; got != affineFromMat4(translated) {
		t.Fatalf("second batch model = %v, want translated view", got)
	}
}

func TestMaterial2DShaderChangeSplitsBatchAndPreservesParameters(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(), width: 320, height: 240, viewMatrix: mgl32.Ident4(), blend: render.PremultipliedAlphaBlend}
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	shader, err := NewMaterial2DShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatalf("create Material2D shader: %v", err)
	}

	context.BindTexture(texture)
	context.BindMaterial2D(shader)
	context.SetMaterial2DParameters([4][4]float32{{0.75}})
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	context.BindMaterial2D(nil)
	context.SetMaterial2DParameters([4][4]float32{})
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	if err := context.FinishFrame(); err != nil {
		t.Fatalf("finish frame: %v", err)
	}

	if len(driver.spriteBatches) != 1 || len(driver.spriteBatches[0].Draws) != 2 {
		t.Fatalf("unexpected Material2D batches: %+v", driver.spriteBatches)
	}
	batch := driver.spriteBatches[0]
	if !batch.Draws[0].Shader.Valid() || batch.Draws[1].Shader.Valid() {
		t.Fatalf("unexpected Material2D shader draws: %+v", batch.Draws)
	}
	if batch.Instances[0].Custom[0][0] != 0.75 || batch.Instances[1].Custom != ([4][4]float32{}) {
		t.Fatalf("unexpected Material2D parameters: %+v", batch.Instances)
	}
}

func TestBindingSameMaterial2DShaderUploadsToReplacementRenderer(t *testing.T) {
	first := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	installCanvasTestDriver(t, first)
	context := &Canvas2D{device: render.ActiveDevice(), width: 320, height: 240, viewMatrix: mgl32.Ident4(), blend: render.PremultipliedAlphaBlend}
	shader, err := NewMaterial2DShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatalf("create Material2D shader: %v", err)
	}
	context.BindMaterial2D(shader)

	render.Uninstall()
	second := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	if err := render.Install(second); err != nil {
		t.Fatalf("install replacement renderer: %v", err)
	}
	context = NewCanvas2D(render.ActiveDevice(), 320, 240)
	context.BindMaterial2D(shader)
	if second.nextShaderID != 1 {
		t.Fatalf("replacement renderer shader uploads = %d, want 1", second.nextShaderID)
	}
}

func TestShapePipelinePreservesSubmissionOrder(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
	installCanvasTestDriver(t, driver)
	context := &Canvas2D{device: render.ActiveDevice(),
		width: 320, height: 240,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
	}
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))

	context.BindTexture(texture)
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	context.DrawShapeMatrices(
		mgl32.Ident4(), mgl32.Ident3(), 20, 10,
		render.ShapeRoundedRect,
		White, ColorRGBA{},
		4, 0, 1,
	)
	context.BindTexture(texture)
	context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	if err := context.FinishFrame(); err != nil {
		t.Fatalf("finish frame: %v", err)
	}

	want := []render.Pipeline{render.PipelineSprite, render.PipelineShape, render.PipelineSprite}
	if len(driver.pipelines) != len(want) {
		t.Fatalf("submission order = %v, want %v", driver.pipelines, want)
	}
	for index := range want {
		if driver.pipelines[index] != want[index] {
			t.Fatalf("submission order = %v, want %v", driver.pipelines, want)
		}
	}
}

func BenchmarkCanvasSpritePacket1000(b *testing.B) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 1920, Height: 1080}, discardBatches: true}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(render.Uninstall)
	context := &Canvas2D{device: render.ActiveDevice(),
		width: 1920, height: 1080,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
		instances:  make([]render.Instance2D, 0, 1000),
		draws:      make([]render.SpriteDraw, 0, 1),
	}
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	context.BindTexture(texture)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for range 1000 {
			context.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
		}
		if err := context.FinishFrame(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCanvasSpriteAffinePacket1000(b *testing.B) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 1920, Height: 1080}, discardBatches: true}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(render.Uninstall)
	context := &Canvas2D{device: render.ActiveDevice(),
		width:      1920,
		height:     1080,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
		instances:  make([]render.Instance2D, 0, 1000),
		draws:      make([]render.SpriteDraw, 0, 1),
	}
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	context.BindTexture(texture)
	model := affineFromMat4(mgl32.Ident4())
	textureModel := affineFromMat3(mgl32.Ident3())
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for range 1000 {
			context.DrawAffine(model, textureModel, 1)
		}
		if err := context.FinishFrame(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCanvasSpritePreparedMatrices1000(b *testing.B) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 1920, Height: 1080}, discardBatches: true}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(render.Uninstall)
	context := &Canvas2D{device: render.ActiveDevice(),
		width:      1920,
		height:     1080,
		viewMatrix: mgl32.Ident4(),
		blend:      render.PremultipliedAlphaBlend,
		instances:  make([]render.Instance2D, 0, 1000),
		draws:      make([]render.SpriteDraw, 0, 1),
	}
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	context.BindTexture(texture)
	model := mgl32.Ident4()
	textureModel := mgl32.Ident3()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for range 1000 {
			context.DrawMatrices(model, textureModel, 1)
		}
		if err := context.FinishFrame(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkApplyViewAffineBatch1000(b *testing.B) {
	benchmarkApplyViewAffineBatch1000(b, applyViewAffineBatch)
}

func BenchmarkApplyViewAffineBatchGo1000(b *testing.B) {
	benchmarkApplyViewAffineBatch1000(b, applyViewAffineBatchGo)
}

func benchmarkApplyViewAffineBatch1000(
	b *testing.B,
	apply func([]render.Instance2D, [6]float32),
) {
	instances := make([]render.Instance2D, 1000)
	for index := range instances {
		instances[index].Model = [6]float32{1, 0, float32(index), 0, 1, float32(-index)}
	}
	view := [6]float32{0.9689124, -0.247404, 0, 0.247404, 0.9689124, 0}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		apply(instances, view)
	}
}

func installCanvasTestDriver(t *testing.T, driver render.Driver) {
	t.Helper()
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatalf("install render driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
}

type canvasTestDriver struct {
	viewport        render.Rect
	scissor         *render.Rect
	beginErr        error
	nextID          uint32
	nextShaderID    uint32
	textures        []render.TextureHandle
	pipelines       []render.Pipeline
	projections     [][16]float32
	submits         int
	spriteBatches   []render.SpriteBatch
	discardBatches  bool
	createdTextures []render.TextureDescriptor
	createdBytes    []int
}

func (d *canvasTestDriver) RendererInfo() render.RendererInfo { return render.RendererInfo{} }
func (d *canvasTestDriver) BeginFrame(render.Color) error     { return d.beginErr }
func (d *canvasTestDriver) SetViewport(viewport render.Rect)  { d.viewport = viewport }
func (d *canvasTestDriver) Viewport() render.Rect             { return d.viewport }
func (d *canvasTestDriver) SetBlend(render.BlendState) error  { return nil }
func (d *canvasTestDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return render.TextureHandle{}, err
	}
	d.createdTextures = append(d.createdTextures, descriptor)
	d.createdBytes = append(d.createdBytes, len(pixels))
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (d *canvasTestDriver) UpdateTextures([]render.TextureUpdate) error {
	return nil
}
func (d *canvasTestDriver) DestroyTexture(render.TextureHandle) error { return nil }
func (d *canvasTestDriver) DrawSprites(batch render.SpriteBatch) error {
	d.submits++
	if d.discardBatches {
		return nil
	}
	d.pipelines = append(d.pipelines, render.PipelineSprite)
	d.projections = append(d.projections, batch.Projection)
	batch.Instances = append([]render.Instance2D(nil), batch.Instances...)
	batch.Draws = append([]render.SpriteDraw(nil), batch.Draws...)
	d.spriteBatches = append(d.spriteBatches, batch)
	for _, draw := range batch.Draws {
		d.textures = append(d.textures, draw.Texture)
	}
	return nil
}
func (d *canvasTestDriver) CreateMaterial2DShader(render.ShaderDescriptor) (render.Material2DShaderHandle, error) {
	d.nextShaderID++
	return render.NewHandle[render.Material2DShaderResource](d.nextShaderID, 1), nil
}
func (d *canvasTestDriver) DestroyMaterial2DShader(render.Material2DShaderHandle) error { return nil }
func (d *canvasTestDriver) DrawShapes(render.ShapeBatch) error {
	d.submits++
	d.pipelines = append(d.pipelines, render.PipelineShape)
	return nil
}
func (d *canvasTestDriver) Present() error               { return nil }
func (d *canvasTestDriver) ResizeSurface(int, int) error { return nil }
func (d *canvasTestDriver) SetVSync(bool)                {}
func (d *canvasTestDriver) Destroy()                     {}

func (d *canvasTestDriver) SetScissor(scissor *render.Rect) {
	if scissor == nil {
		d.scissor = nil
		return
	}
	copy := *scissor
	d.scissor = &copy
}
