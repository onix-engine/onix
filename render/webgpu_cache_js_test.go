//go:build js && wasm && !wechat

package render

import (
	"syscall/js"
	"testing"
)

func newWebGPUCacheTestDriver() (*webGPUDriver, js.Value) {
	mock := js.Global().Get("Function").New(`
 const stats = {created:0};
 return {stats, device:{ createBindGroup(descriptor) { stats.created++; return descriptor; }, destroy() {} },
  frame:{setBindGroup() {}, begin() {}, setViewport() {}, setScissorRect() {}, destroy() {} }};
 `).Invoke()
	d := &webGPUDriver{device: mock.Get("device"), frame: mock.Get("frame"), dynamicOffsets: webGPUArray(0),
		emptyDeformation: webGPUObject(), clearValue: webGPUObject(), sampleCount: 1, width: 16, height: 16,
		frameIndex: webGPUFramesInFlight - 1}
	for i := range d.frames {
		d.frames[i].buffer.value = webGPUObject("destroy", mock.Get("device").Get("destroy"))
		d.frames[i].generation = 1
	}
	d.emptyDeformation.Set("destroy", mock.Get("device").Get("destroy"))
	return d, mock.Get("stats")
}

func TestWebGPUBindingCachesExpireThroughBeginFrame(t *testing.T) {
	d, stats := newWebGPUCacheTestDriver()
	d.frameIndex = 0
	hot, cold := &webGPUTexture{}, &webGPUTexture{}
	hotMesh, coldMesh := &webGPUMesh{deformationBuffer: webGPUObject()}, &webGPUMesh{deformationBuffer: webGPUObject()}
	var extras [MeshExtraTextureSlots]*webGPUTexture
	bindMaterial := func(texture *webGPUTexture) {
		for i := range extras {
			extras[i] = texture
		}
		d.setMeshTextureBindGroup(texture, texture, texture, extras, 0)
	}
	bindMaterial(cold)
	d.deformationSize = 64
	d.setMeshDataBindGroup(coldMesh)
	d.setMeshDataBindGroup(nil)
	oldMaterial := cold.materials.binding
	oldData := coldMesh.bindGroups
	for frame := 0; frame < frameCacheIdleFrames+4; frame++ {
		// The mock has no GPU pass to submit; BeginFrame is the actual cache clock.
		d.frameActive = false
		if err := d.BeginFrame(Color{}); err != nil {
			t.Fatal(err)
		}
		bindMaterial(hot)
		d.deformationSize = 128
		d.setMeshDataBindGroup(hotMesh)
	}
	if len(d.meshBindGroups.entries) != 1 || len(d.meshDataGroups.entries) != 1 || cold.materials != nil || coldMesh.bindGroups != nil {
		t.Fatal("cold material, mesh or default binding survived eviction")
	}
	if oldMaterial.value.groups[0].value.Truthy() || oldData.value.groups[0].value.Truthy() || oldMaterial.key.base != nil || oldData.key.mesh != nil {
		t.Fatal("evicted entries retain JS groups or resource owners")
	}
	if stats.Get("created").Int() != 3+webGPUFramesInFlight*2 {
		t.Fatal("hot bindings were recreated")
	}
	bindMaterial(cold)
	d.deformationSize = 64
	d.setMeshDataBindGroup(coldMesh)
	if stats.Get("created").Int() != 5+webGPUFramesInFlight*2 {
		t.Fatal("evicted bindings did not rebuild")
	}
	// Old frame-buffer generations still invalidate a live cache entry.
	d.frames[d.frameIndex].generation++
	bindMaterial(cold)
	d.setMeshDataBindGroup(coldMesh)
	if stats.Get("created").Int() != 7+webGPUFramesInFlight*2 {
		t.Fatal("buffer generation did not invalidate cached bindings")
	}
}

func TestWebGPUBindingCachePeakReleasesOwnersAndPreservesHotGroups(t *testing.T) {
	d, stats := newWebGPUCacheTestDriver()
	d.frameIndex = 0
	base := &webGPUTexture{}
	owners := make([]webGPUTexture, 1024)
	meshes := make([]webGPUMesh, 2)
	var extra [MeshExtraTextureSlots]*webGPUTexture
	for i := range extra {
		extra[i] = base
	}
	for i := range owners {
		d.setMeshTextureBindGroup(&owners[i], base, base, extra, 0)
		d.deformationSize = (i + 1) * 4
		d.setMeshDataBindGroup(&meshes[i%2])
	}
	initial := stats.Get("created").Int()
	hotMaterial := owners[0].materials.binding
	hotData := d.meshDataGroups.entries[webGPUDataKey{mesh: &meshes[0], size: 4}]
	for frame := uint64(1); frame <= frameCacheIdleFrames+bufferTrimDelay+32; frame++ {
		d.cacheFrame = frame
		d.setMeshTextureBindGroup(&owners[0], base, base, extra, 0)
		d.deformationSize = 4
		d.setMeshDataBindGroup(&meshes[0])
		d.trimBindingCaches()
	}
	if len(d.meshBindGroups.entries) != 1 || len(d.meshDataGroups.entries) != 1 || d.meshBindGroups.capacity != 32 || d.meshDataGroups.capacity != 32 {
		t.Fatalf("retained cache peak: %d materials/%d data groups", len(d.meshBindGroups.entries), len(d.meshDataGroups.entries))
	}
	if owners[0].materials.binding != hotMaterial || meshes[0].bindGroups != hotData || stats.Get("created").Int() != initial {
		t.Fatal("shrinking recreated the hot binding")
	}
	for i := 1; i < len(owners); i++ {
		if owners[i].materials != nil {
			t.Fatal("cold owner still links to an evicted material")
		}
	}
	if meshes[1].bindGroups != nil {
		t.Fatal("cold mesh still links to an evicted data group")
	}
	d.deleteTexture(base, false)
	if owners[0].materials != nil || len(d.meshBindGroups.entries) != 0 {
		t.Fatal("texture destruction after eviction left peer links")
	}
	d.meshDataGroups.clear(unlinkWebGPUData)
	if meshes[0].bindGroups != nil {
		t.Fatal("cache disposal left mesh links")
	}
}

func BenchmarkWebGPUBindingCacheWarm(b *testing.B) {
	d, stats := newWebGPUCacheTestDriver()
	texture := &webGPUTexture{}
	mesh := &webGPUMesh{deformationBuffer: webGPUObject()}
	var extra [MeshExtraTextureSlots]*webGPUTexture
	for i := range extra {
		extra[i] = texture
	}
	d.deformationSize = 64
	for slot := range d.frames {
		d.frameIndex = slot
		d.setMeshTextureBindGroup(texture, texture, texture, extra, 0)
		d.setMeshDataBindGroup(mesh)
	}
	created := stats.Get("created").Int()
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		d.cacheFrame++
		d.frameIndex = i % webGPUFramesInFlight
		d.setMeshTextureBindGroup(texture, texture, texture, extra, 0)
		d.setMeshDataBindGroup(mesh)
		d.trimBindingCaches()
	}
	b.StopTimer()
	b.ReportMetric(float64(stats.Get("created").Int()-created)/float64(b.N), "jsbindgroups/op")
}

func TestWebGPUDestroyClearsBindingOwners(t *testing.T) {
	d, _ := newWebGPUCacheTestDriver()
	texture := &webGPUTexture{}
	mesh := &webGPUMesh{}
	var extra [MeshExtraTextureSlots]*webGPUTexture
	for i := range extra {
		extra[i] = texture
	}
	d.setMeshTextureBindGroup(texture, texture, texture, extra, 0)
	d.deformationSize = 4
	d.setMeshDataBindGroup(mesh)
	material, data := texture.materials.binding, mesh.bindGroups
	d.Destroy()
	if texture.materials != nil || mesh.bindGroups != nil || d.meshBindGroups.entries != nil || d.meshDataGroups.entries != nil {
		t.Fatal("driver disposal retained cache owners or map storage")
	}
	if material.key.base != nil || data.key.mesh != nil {
		t.Fatal("disposed entries still reference their owners")
	}
}
