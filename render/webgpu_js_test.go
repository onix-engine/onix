//go:build js && wasm && !wechat

package render

import (
	"strings"
	"syscall/js"
	"testing"
)

func TestWebGPURejectsInsecureContext(t *testing.T) {
	global := js.Global()
	secure := global.Get("isSecureContext")
	global.Set("isSecureContext", false)
	defer func() {
		if secure.IsUndefined() {
			global.Delete("isSecureContext")
		} else {
			global.Set("isSecureContext", secure)
		}
	}()
	driver, err := NewWebGPUDriver(webGPUObject(), false, false)
	if driver != nil || err == nil || !strings.Contains(err.Error(), "HTTPS or http://localhost") {
		t.Fatalf("insecure context: driver=%v error=%v", driver, err)
	}
}

func TestWebGPUCustomMeshPipelinesAreCreatedOnDemand(t *testing.T) {
	modules, pipelines := 0, 0
	module := js.FuncOf(func(_ js.Value, args []js.Value) any { modules++; return args[0] })
	defer module.Release()
	pipeline := js.FuncOf(func(_ js.Value, args []js.Value) any { pipelines++; return args[0] })
	defer pipeline.Release()
	d := webGPUDriver{
		device:      webGPUObject("createShaderModule", module, "createRenderPipeline", pipeline),
		frameActive: true, format: "rgba8unorm", sampleCount: 1,
	}
	handle, err := d.CreateMeshShader(ShaderDescriptor{WebGPUVertexSource: webGPUMeshVertex, WebGPUFragmentSource: webGPUMeshFragment})
	if err != nil {
		t.Fatal(err)
	}
	if modules != 2 || pipelines != 0 {
		t.Fatalf("shader creation compiled %d modules and %d pipelines", modules, pipelines)
	}
	shader, _ := d.meshShaders.Get(handle)
	for _, transparent := range []int{0, 0, 1, 1} {
		if _, err := d.meshPipeline(shader, transparent, MeshBlendNormal); err != nil {
			t.Fatal(err)
		}
	}
	if pipelines != 2 {
		t.Fatalf("used variants compiled %d pipelines, want 2", pipelines)
	}
	for mode := MeshBlendAdditive; mode <= MeshBlendScreen; mode++ {
		if shader.meshPipelines[0][int(mode)+1].Truthy() {
			t.Fatal("unused blend variant was compiled")
		}
	}
	if err := d.DestroyMeshShader(handle); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.meshShaders.Get(handle); ok {
		t.Fatal("destroyed shader remains resident")
	}
}

func TestWebGPULCDColorsSharePipeline(t *testing.T) {
	pipelines := 0
	module := js.FuncOf(func(_ js.Value, args []js.Value) any { return args[0] })
	defer module.Release()
	create := js.FuncOf(func(_ js.Value, args []js.Value) any { pipelines++; return args[0] })
	defer create.Release()
	d := webGPUDriver{
		device:      webGPUObject("createShaderModule", module, "createRenderPipeline", create),
		frameActive: true, format: "rgba8unorm", sampleCount: 1,
		blendConstant: webGPUObject("r", 0, "g", 0, "b", 0, "a", 0),
	}
	shader, err := d.newShader(PipelineSprite, webGPUSpriteVertex, webGPUSpriteFragment)
	if err != nil {
		t.Fatal(err)
	}
	for _, color := range []Color{{R: 1}, {G: 1}, {B: 1}} {
		blend := LCDTextBlend
		blend.Constant = color
		if err := d.SetBlend(blend); err != nil {
			t.Fatal(err)
		}
		if _, err := d.pipeline(shader, blend); err != nil {
			t.Fatal(err)
		}
		if d.blendConstant.Get("r").Float() != float64(color.R) ||
			d.blendConstant.Get("g").Float() != float64(color.G) || d.blendConstant.Get("b").Float() != float64(color.B) {
			t.Fatal("LCD foreground constant was not updated")
		}
	}
	if pipelines != 1 {
		t.Fatalf("three text colors created %d pipelines, want one", pipelines)
	}
}

func TestWebGPUSpriteLayoutsKeepCustomInputsAndClipOffsets(t *testing.T) {
	var d webGPUDriver
	for _, custom := range []bool{false, true} {
		layout := d.spriteVertexLayouts(custom).Index(1)
		if layout.Get("arrayStride").Int() != spriteInstanceStride(custom) {
			t.Fatal("incorrect sprite stride")
		}
		attributes := layout.Get("attributes")
		for index := 0; index < attributes.Length(); index++ {
			attribute := attributes.Index(index)
			location := attribute.Get("shaderLocation").Int()
			if attribute.Get("offset").Int() != spriteAttributeOffset(location, custom) {
				t.Fatalf("incorrect attribute %d (custom=%v)", location, custom)
			}
		}
	}
}

func TestWebGPUTextureBindGroupMatchesLayout(t *testing.T) {
	createLayout := js.FuncOf(func(_ js.Value, args []js.Value) any { return args[0] })
	defer createLayout.Release()
	creations := 0
	createGroup := js.FuncOf(func(_ js.Value, args []js.Value) any {
		creations++
		return args[0]
	})
	defer createGroup.Release()
	d := webGPUDriver{device: webGPUObject("createBindGroupLayout", createLayout, "createBindGroup", createGroup)}
	d.textureLayout, d.shapeLayout = d.createBindGroupLayouts()
	texture := webGPUTexture{view: webGPUObject(), sampler: webGPUObject()}
	for index := range d.frames {
		d.frameIndex = index
		frame := &d.frames[index]
		for generation := uint64(1); generation <= 2; generation++ {
			frame.generation = generation
			frame.buffer.value = webGPUObject()
			group := d.textureBindGroup(&texture)
			if !group.Get("layout").Equal(d.textureLayout) {
				t.Fatal("texture bind group uses a different layout")
			}
			entries, layout := group.Get("entries"), d.textureLayout.Get("entries")
			if entries.Length() != layout.Length() {
				t.Fatalf("bind group has %d entries, layout requires %d", entries.Length(), layout.Length())
			}
			for binding := range layout.Length() {
				entry, expected := entries.Index(binding), layout.Index(binding)
				if entry.Get("binding").Int() != expected.Get("binding").Int() {
					t.Fatalf("entry %d does not match its layout binding", binding)
				}
				resource := entry.Get("resource")
				switch {
				case expected.Get("texture").Truthy():
					if !resource.Equal(texture.view) {
						t.Fatalf("binding %d does not use the texture view", binding)
					}
				case expected.Get("sampler").Truthy():
					if !resource.Equal(texture.sampler) {
						t.Fatalf("binding %d does not use the texture sampler", binding)
					}
				case expected.Get("buffer").Truthy():
					if !resource.Get("buffer").Equal(frame.buffer.value) {
						t.Fatalf("binding %d does not use the current frame buffer", binding)
					}
				}
			}
			before := creations
			if !d.textureBindGroup(&texture).Equal(group) || creations != before {
				t.Fatal("texture bind group was recreated within the same frame buffer generation")
			}
			if creations != index*2+int(generation) {
				t.Fatal("texture bind group was not recreated for the new frame buffer generation")
			}
		}
	}
}

func TestWebGPUMeshBindingsReuseAcrossAnimatedOffsets(t *testing.T) {
	creations := 0
	create := js.FuncOf(func(_ js.Value, args []js.Value) any { creations++; return args[0] })
	defer create.Release()
	bind := js.FuncOf(func(_ js.Value, args []js.Value) any { return nil })
	defer bind.Release()
	d := webGPUDriver{device: webGPUObject("createBindGroup", create), frame: webGPUObject("setBindGroup", bind), dynamicOffsets: webGPUArray(0), meshTextureLayout: webGPUObject(), meshDataLayout: webGPUObject(), emptyDeformation: webGPUObject()}
	texture := &webGPUTexture{view: webGPUObject(), sampler: webGPUObject()}
	mesh := &webGPUMesh{deformationBuffer: webGPUObject()}
	var extra [MeshExtraTextureSlots]*webGPUTexture
	for slot := range extra {
		extra[slot] = texture
	}
	for frame := range d.frames {
		d.frameIndex = frame
		d.frames[frame].generation = 1
		d.frames[frame].buffer.value = webGPUObject()
		for repeat := range 3 {
			for batch := range 8 {
				d.deformationOffset = 256 * (batch + 1)
				d.deformationSize = 64
				d.setMeshTextureBindGroup(texture, texture, texture, extra, 256*(batch+16))
				d.setMeshDataBindGroup(mesh)
				if d.dynamicOffsets.Index(0).Int() != d.deformationOffset {
					t.Fatal("animated offset was not supplied dynamically")
				}
			}
			if creations != (frame+1)*2 {
				t.Fatalf("repeat %d recreated bindings: %d", repeat, creations)
			}
		}
	}
	d.frames[d.frameIndex].generation++
	d.setMeshTextureBindGroup(texture, texture, texture, extra, 0)
	d.setMeshDataBindGroup(mesh)
	if creations != 8 {
		t.Fatalf("buffer growth failed to invalidate groups: %d", creations)
	}
	d.deleteTexture(texture, false)
	if len(d.meshBindGroups.entries) != 0 {
		t.Fatal("destroyed texture remains cached")
	}
}

func TestWebGPUSpriteUniformsShareCoverageMasksAndOneUpload(t *testing.T) {
	writes := 0
	write := js.FuncOf(func(_ js.Value, args []js.Value) any { writes++; return nil })
	defer write.Release()
	d := webGPUDriver{queue: webGPUObject("writeBuffer", write)}
	d.frames[0] = webGPUFrame{buffer: webGPUBuffer{value: webGPUObject(), capacity: 65536}, generation: 1}
	rgba := d.textures.Insert(&webGPUTexture{format: TextureFormatRGBA8})
	coverage := d.textures.Insert(&webGPUTexture{format: TextureFormatR8})
	draws := make([]SpriteDraw, 64)
	for i := range draws {
		draws[i].InstanceCount = 1
		draws[i].Texture = rgba
		draws[i].AdditionalTextures = [SpriteTextureSlots - 1]TextureHandle{rgba, rgba, rgba}
		mask := i % 16
		for slot := range SpriteTextureSlots {
			if mask&(1<<slot) != 0 {
				if slot == 0 {
					draws[i].Texture = coverage
				} else {
					draws[i].AdditionalTextures[slot-1] = coverage
				}
			}
		}
	}
	projection := [16]float32{0: 1, 5: 1, 10: 1, 15: 1}
	for range 2 {
		d.frames[0].offset = 0
		writes = 0
		_, offsets := d.uploadSprite2D(projection, make([]byte, 6400), draws)
		if writes != 2 || d.frames[0].offset != 6400+16*256 {
			t.Fatalf("writes=%d reserved=%d", writes, d.frames[0].offset)
		}
		for i, offset := range offsets {
			if offset != offsets[i%16] {
				t.Fatal("equivalent coverage did not share uniforms")
			}
			uniform := d.spriteUniformData[(offset-6400)/4:]
			for slot := range SpriteTextureSlots {
				if uniform[16+slot] != float32((i%16>>slot)&1) {
					t.Fatal("coverage flags changed")
				}
			}
		}
	}
}

func TestWebGPUMeshDestructionRetiresUnsubmittedBuffers(t *testing.T) {
	for _, active := range [...]bool{false, true} {
		destroyed := 0
		destroy := js.FuncOf(func(_ js.Value, _ []js.Value) any { destroyed++; return nil })
		d := webGPUDriver{frameActive: active}
		mesh := &webGPUMesh{vertexBuffer: webGPUObject("destroy", destroy), indexBuffer: webGPUObject("destroy", destroy), deformationBuffer: webGPUObject("destroy", destroy)}
		handle := d.meshes.Insert(mesh)
		d.device = webGPUObject("createBindGroup", js.Global().Get("Function").New("return {};"))
		d.frame = webGPUObject("setBindGroup", js.Global().Get("Function").New(""))
		d.dynamicOffsets = webGPUArray(0)
		for _, size := range []int{64, 128} {
			d.deformationSize = size
			d.setMeshDataBindGroup(mesh)
		}
		d.deformationSize = 4
		d.setMeshDataBindGroup(nil)
		if err := d.DestroyMesh(handle); err != nil {
			t.Fatal(err)
		}
		if _, ok := d.meshes.Get(handle); ok || mesh.bindGroups != nil || len(d.meshDataGroups.entries) != 1 {
			t.Fatal("destroyed mesh or its bindings remain cached")
		}
		if active && (destroyed != 0 || len(d.frames[0].retired) != 3) || !active && (destroyed != 3 || len(d.frames[0].retired) != 0) {
			t.Fatalf("active=%v destroyed=%d retired=%d", active, destroyed, len(d.frames[0].retired))
		}
		if err := d.DestroyMesh(handle); err == nil {
			t.Fatal("duplicate mesh destruction accepted")
		}
		destroy.Release()
	}
}

func TestWebGPUTextureDestructionKeepsUnrelatedBindings(t *testing.T) {
	creations := 0
	create := js.FuncOf(func(_ js.Value, args []js.Value) any { creations++; return args[0] })
	defer create.Release()
	bind := js.FuncOf(func(_ js.Value, _ []js.Value) any { return nil })
	defer bind.Release()
	d := webGPUDriver{
		device: webGPUObject("createBindGroup", create), frame: webGPUObject("setBindGroup", bind),
		dynamicOffsets:    webGPUArray(0),
		meshTextureLayout: webGPUObject(),
	}
	d.frames[0].generation = 1
	d.frames[0].buffer.value = webGPUObject()
	base, unrelated := &webGPUTexture{}, &webGPUTexture{}
	var extra [MeshExtraTextureSlots]*webGPUTexture
	for index := range extra {
		extra[index] = &webGPUTexture{}
	}
	d.setMeshTextureBindGroup(base, base, base, extra, 0)
	related := extra
	related[len(related)-1] = base
	d.setMeshTextureBindGroup(base, base, base, related, 0)
	var independent [MeshExtraTextureSlots]*webGPUTexture
	for index := range independent {
		independent[index] = unrelated
	}
	d.setMeshTextureBindGroup(unrelated, unrelated, unrelated, independent, 0)
	d.deleteTexture(extra[len(extra)-1], false)
	if len(d.meshBindGroups.entries) != 2 || base.materials == nil {
		t.Fatal("destroyed extra texture left material bindings behind or removed unrelated bindings")
	}
	for _, texture := range extra {
		for link := texture.materials; link != nil; link = link.next {
			if d.meshBindGroups.entries[link.binding.key] != link.binding {
				t.Fatal("a material peer retains a removed binding")
			}
		}
	}
	d.setMeshTextureBindGroup(unrelated, unrelated, unrelated, independent, 0)
	if creations != 3 {
		t.Fatal("unrelated material lost its cached binding")
	}
	d.deleteTexture(base, false)
	if len(d.meshBindGroups.entries) != 1 {
		t.Fatal("destroying a former material peer removed an unrelated binding")
	}
	for _, texture := range extra {
		if texture.materials != nil {
			t.Fatal("removing the shared base texture did not unlink its material peers")
		}
	}
	d.deleteTexture(unrelated, false)
	if len(d.meshBindGroups.entries) != 0 || unrelated.materials != nil {
		t.Fatal("duplicated texture slots prevent complete cache cleanup")
	}
}
