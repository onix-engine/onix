package project

type ModContentFeatures uint16

const (
	ModContentAnimations ModContentFeatures = 1 << iota
	ModContentLayout
	ModContentTrail
	ModContentHTML
	ModContentParticles
	ModContentTilemaps
	ModContentSpine
	ModContentSpatial
	ModContentPhysics2D
	ModContentAll = (ModContentPhysics2D << 1) - 1
)

func (c ModComponents) ContentFeatures() (features ModContentFeatures) {
	if c.Animation != nil {
		features |= ModContentAnimations
	}
	if c.Layout != nil {
		features |= ModContentLayout
	}
	if c.Trail != nil {
		features |= ModContentTrail
	}
	if c.HTML != nil {
		features |= ModContentHTML
	}
	if c.Emitter != nil || c.Particle != nil {
		features |= ModContentParticles
	}
	if c.Tilemap != nil {
		features |= ModContentTilemaps
	}
	if c.Spine != nil {
		features |= ModContentSpine
	}
	if c.Model != nil || c.Camera != nil || c.AmbientLight != nil || c.DirectionalLight != nil || c.PointLight != nil || c.SpotLight != nil || c.AreaLight != nil || c.RigidBody3D != nil || c.TriangleMeshCollider3D != nil || c.ConvexHullCollider3D != nil || c.CompoundCollider3D != nil {
		features |= ModContentSpatial
	}
	if c.RigidBody2D != nil || c.CircleCollider2D != nil || c.BoxCollider2D != nil || c.SegmentCollider2D != nil || c.Collision != nil && c.RigidBody3D == nil {
		features |= ModContentPhysics2D
	}
	return
}
