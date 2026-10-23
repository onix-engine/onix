package project

// EmitterSpec is a module component that spawns particles. It carries only
// emission behaviour; the sprite and colour of each particle come from the
// elements named by Particles, whose particle components declare textures.
type EmitterSpec struct {
	Particles  []string `json:"particles"`
	Width      float32  `json:"width"`
	Height     float32  `json:"height"`
	Count      int      `json:"count"`
	Rate       float32  `json:"rate"`
	Lifetime   float32  `json:"lifetime"`
	SpeedMin   float32  `json:"speedMin"`
	SpeedMax   float32  `json:"speedMax"`
	SizeMin    float32  `json:"sizeMin"`
	SizeMax    float32  `json:"sizeMax"`
	Direction  float32  `json:"direction"`
	Spread     float32  `json:"spread"`
	GravityX   float32  `json:"gravityX"`
	GravityY   float32  `json:"gravityY"`
	AlphaStart float32  `json:"alphaStart"`
	AlphaEnd   float32  `json:"alphaEnd"`
	Loop       bool     `json:"loop"`
}

type ParticleSpec struct {
	ID         string       `json:"id"`
	Width      float32      `json:"width"`
	Height     float32      `json:"height"`
	Texture    *TextureSpec `json:"texture"`
	Colors     []string     `json:"colors"`
	Count      int          `json:"count"`
	Rate       float32      `json:"rate"`
	Lifetime   float32      `json:"lifetime"`
	SpeedMin   float32      `json:"speedMin"`
	SpeedMax   float32      `json:"speedMax"`
	SizeMin    float32      `json:"sizeMin"`
	SizeMax    float32      `json:"sizeMax"`
	Direction  float32      `json:"direction"`
	Spread     float32      `json:"spread"`
	GravityX   float32      `json:"gravityX"`
	GravityY   float32      `json:"gravityY"`
	AlphaStart float32      `json:"alphaStart"`
	AlphaEnd   float32      `json:"alphaEnd"`
	Loop       bool         `json:"loop"`
}
