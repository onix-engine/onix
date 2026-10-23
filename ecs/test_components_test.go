package ecs

// Core tests use ordinary application components without importing the scene
// runtime, so these tests also exercise the ECS as an independent library.
type testName struct{ Value string }
type testVisibility struct{ Visible bool }
type testSize struct{ Width, Height float32 }
type testParent struct{ Entity Entity }
type testAnchor [2]float32
type testStack int

var (
	testAnchorCenter  = testAnchor{}
	testAnchorTopLeft = testAnchor{-0.5, 0.5}
)
