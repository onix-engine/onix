package render

import "testing"

func TestFrameStatisticsAccumulateSubmittedWork(t *testing.T) {
	device := &Device{statistics: &driverStatistics{}}
	device.Activate()
	t.Cleanup(device.Destroy)
	state := device.statistics
	state.beginFrameStatistics(1920, 1080)
	state.recordSpriteStatistics(SpriteBatch{
		Instances: make([]Instance2D, 5),
		Draws: []SpriteDraw{
			{FirstInstance: 0, InstanceCount: 2},
			{FirstInstance: 2, InstanceCount: 3},
		},
	}, 5, 500)
	state.recordShapeStatistics(ShapeBatch{Instances: make([]ShapeInstance2D, 2)})
	state.recordMeshStatistics(MeshBatch3D{
		Vertices:  make([]Vertex3D, 4),
		Indices:   make([]uint32, 9),
		Instances: make([]Instance3D, 3),
		Draws: []MeshDraw3D{
			{IndexCount: 6, InstanceCount: 2},
			{IndexCount: 3, InstanceCount: 1},
		},
	}, meshDrawStatistics{instances: 3, vertices: 15, triangles: 5})
	state.recordTextureUploadStatistics(2, 128)

	got := Statistics()
	want := FrameStatistics{
		SurfaceWidth:           1920,
		SurfaceHeight:          1080,
		DrawCalls:              5,
		Sprites:                5,
		Shapes:                 2,
		Meshes:                 3,
		Vertices:               43,
		Triangles:              19,
		SpriteDrawCalls:        2,
		ShapeDrawCalls:         1,
		MeshDrawCalls:          2,
		BufferUploadBytes:      1504,
		DynamicMeshDrawCalls:   2,
		DynamicMeshVertices:    4,
		DynamicMeshIndices:     9,
		DynamicMeshUploadBytes: 228,
		TextureUploads:         2,
		TextureUploadBytes:     128,
	}
	if got != want {
		t.Fatalf("statistics = %+v, want %+v", got, want)
	}

	ResetStatistics()
	if got := Statistics(); got != (FrameStatistics{}) {
		t.Fatalf("statistics after reset = %+v", got)
	}
}

func TestFrameStatisticsRecordingDoesNotAllocate(t *testing.T) {
	device := &Device{statistics: &driverStatistics{}}
	device.Activate()
	t.Cleanup(device.Destroy)
	state := device.statistics
	sprite := SpriteBatch{
		Instances: make([]Instance2D, 4),
		Draws:     []SpriteDraw{{InstanceCount: 4}},
	}
	shape := ShapeBatch{Instances: make([]ShapeInstance2D, 2)}
	mesh := MeshBatch3D{
		Vertices:  make([]Vertex3D, 3),
		Indices:   make([]uint32, 3),
		Instances: make([]Instance3D, 1),
		Draws:     []MeshDraw3D{{IndexCount: 3, InstanceCount: 1}},
	}
	allocations := testing.AllocsPerRun(100, func() {
		state.beginFrameStatistics(1280, 720)
		state.recordSpriteStatistics(sprite, 4, 400)
		state.recordShapeStatistics(shape)
		state.recordMeshStatistics(mesh, meshDrawStatistics{instances: 1, vertices: 3, triangles: 1})
		state.recordTextureUploadStatistics(1, 16)
		_ = Statistics()
	})
	if allocations != 0 {
		t.Fatalf("statistics recording allocated %.0f objects, want 0", allocations)
	}
}

func TestMeshDrawStatisticsPreservesPerDrawTriangleRounding(t *testing.T) {
	var counts meshDrawStatistics
	counts.add(MeshDraw3D{IndexCount: 5, InstanceCount: 2})
	counts.add(MeshDraw3D{IndexCount: 4, InstanceCount: 3})
	if counts != (meshDrawStatistics{instances: 5, vertices: 22, triangles: 5}) {
		t.Fatalf("counts=%+v", counts)
	}
}
