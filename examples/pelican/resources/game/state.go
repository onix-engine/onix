package game

import (
	"math"
	"slices"
)

const (
	routeLength float32 = 15600
	riderX      float32 = 270
	capacity            = 6
	target              = 18
)

type phase uint8

const (
	ready phase = iota
	riding
	paused
	finished
	crashed
)

type kind uint8

const (
	fish kind = iota
	crab
	crate
	station
	finish
)

type object struct {
	X, Height float32
	Kind      kind
	Used      bool
}
type particle struct{ X, Y, VX, VY, Life float32 }

// All moving state is bounded. Course construction and simulation allocate nothing.
type game struct {
	Phase                                               phase
	Distance, Height, Velocity, Speed, Elapsed          float32
	Boost, Invincible, NoticeTime, DeliveryTime         float32
	Cargo, Delivered, Score, Best, Hearts, Combo, Stops int
	Held, Flapped, Gliding                              bool
	Notice                                              string
	Objects                                             [112]object
	Count, First                                        int
	Particles                                           [12]particle
	particleNext                                        int
	DeliveryX, DeliveryY                                float32
}

func (g *game) reset() {
	best := max(g.Best, g.Score)
	*g = game{Best: best, Hearts: 3, Speed: 245, Notice: "今天的任务：把 18 条鲜鱼送到海岸小站。"}
	add := func(x, h float32, k kind) { g.Objects[g.Count] = object{X: x, Height: h, Kind: k}; g.Count++ }
	for block := 0; block < 15; block++ {
		x := float32(block)*1000 + 650
		for i, h := range [...]float32{125, 185, 225, 185} {
			add(x+float32(i)*80, h, fish)
		}
		k := crab
		if block%3 == 2 {
			k = crate
		}
		add(x+170, 0, k)
		// Later districts add isolated hazards with ample recovery distance.
		if block >= 5 {
			add(x+650, 0, crab)
		}
	}
	for i := 1; i <= 6; i++ {
		add(float32(i)*2500, 0, station)
	}
	add(routeLength+150, 0, finish)
	slices.SortFunc(g.Objects[:g.Count], func(a, b object) int {
		if a.X < b.X {
			return -1
		}
		if a.X > b.X {
			return 1
		}
		return 0
	})
}

func (g *game) start() {
	g.reset()
	g.Phase = riding
	g.say("出发！长按跳跃可以展开翅膀滑翔。")
}
func (g *game) say(s string) { g.Notice = s; g.NoticeTime = 3.2 }
func (g *game) togglePause() {
	if g.Phase == riding {
		g.Phase = paused
		g.Held = false
		g.Gliding = false
	} else if g.Phase == paused {
		g.Phase = riding
	}
}
func (g *game) jump() {
	if g.Phase != riding || g.Held {
		return
	}
	g.Held = true
	if g.Height <= 0 {
		g.Velocity = 660 - float32(g.Cargo)*12
		g.Flapped = false
	} else if !g.Flapped && g.Cargo > 0 {
		g.Cargo--
		g.Flapped = true
		g.Velocity = 430
		g.say("借鱼起飞！消耗 1 条鱼，再扑一次翅膀。")
		g.burst(riderX, 606-g.Height-140)
	}
}
func (g *game) release() { g.Held = false }
func (g *game) nearStation() int {
	for i := g.First; i < g.Count; i++ {
		o := &g.Objects[i]
		if o.X-g.Distance > riderX+1100 {
			break
		}
		if o.Kind == station && !o.Used && abs(o.X-g.Distance-riderX) < 205 {
			return i
		}
	}
	return -1
}
func (g *game) deliver() bool {
	if g.Phase != riding {
		return false
	}
	i := g.nearStation()
	if i < 0 {
		g.say("靠近灯塔小站时，按 X 投递。金色提示会亮起。")
		return false
	}
	if g.Cargo == 0 {
		g.say("嘴袋还是空的，先去接住空中的鱼吧！")
		return false
	}
	o := &g.Objects[i]
	o.Used = true
	g.DeliveryX = o.X + 45
	g.DeliveryY = 606 - g.Height - 155
	g.DeliveryTime = .65
	g.Delivered += g.Cargo
	g.Stops++
	g.Combo++
	g.Score += g.Cargo*100 + g.Combo*50
	if g.Cargo == capacity {
		g.Score += 200
		g.say("满袋鲜鱼！额外 +200，顺风冲刺！")
	} else {
		g.say("投递成功！嘴袋清空，顺风冲刺！")
	}
	g.Cargo = 0
	g.Boost = 3.5
	g.burst(g.DeliveryX-g.Distance, 460)
	return true
}
func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
func (g *game) burst(x, y float32) {
	for i := 0; i < 6; i++ {
		p := &g.Particles[g.particleNext%len(g.Particles)]
		g.particleNext++
		a := float64(i) * math.Pi / 3
		*p = particle{X: x, Y: y, VX: float32(math.Cos(a)) * 100, VY: float32(math.Sin(a))*100 - 65, Life: .65}
	}
}
func (g *game) wind() bool { return g.Distance >= 5200 && int(g.Distance/1300)%3 == 1 }
func (g *game) step(dt float32) {
	if g.Phase != riding || dt <= 0 {
		return
	}
	// The engine calls this at fixed ticks. Bound a pathological host delta.
	dt = min(dt, 1.0/30)
	g.Elapsed += dt
	g.NoticeTime = max(0, g.NoticeTime-dt)
	g.Invincible = max(0, g.Invincible-dt)
	g.Boost = max(0, g.Boost-dt)
	g.DeliveryTime = max(0, g.DeliveryTime-dt)
	speed := float32(245) + min(g.Distance/220, 60) - float32(g.Cargo)*3
	if g.wind() {
		speed -= 45
	}
	if g.Boost > 0 {
		speed += 125
	}
	g.Speed = speed
	g.Distance += speed * dt
	gravity := float32(1550) + float32(g.Cargo)*30
	g.Gliding = g.Held && g.Height > 10 && g.Velocity < 0
	if g.Gliding {
		gravity = 330
		g.Velocity = max(g.Velocity, -110)
		g.Distance += 40 * dt
	}
	g.Velocity -= gravity * dt
	g.Height += g.Velocity * dt
	if g.Height <= 0 {
		g.Height = 0
		g.Velocity = 0
		g.Flapped = false
		g.Gliding = false
	}
	for g.First < g.Count && g.Objects[g.First].X < g.Distance-180 {
		g.First++
	}
	// One traversal handles both pickups and obstacle contacts.
	for i := g.First; i < g.Count; i++ {
		o := &g.Objects[i]
		dx := o.X - g.Distance - riderX
		if dx > 1100 {
			break
		}
		if o.Used {
			continue
		}
		switch o.Kind {
		case fish:
			if abs(dx-65) < 76 && abs(o.Height-(g.Height+135)) < 67 && g.Cargo < capacity {
				o.Used = true
				g.Cargo++
				g.Score += 10
				g.burst(o.X-g.Distance, 606-o.Height)
				if g.Cargo == capacity {
					g.say("嘴袋满啦！找一座灯塔，把鲜鱼送出去。")
				}
			}
		case crab, crate:
			clearance, radius := float32(43), float32(44)
			if o.Kind == crate {
				clearance, radius = 64, 36
			}
			if abs(dx) < radius && g.Height < clearance && g.Invincible == 0 {
				o.Used = true
				g.Hearts--
				g.Cargo = max(0, g.Cargo-2)
				g.Combo = 0
				g.Invincible = 1.8
				g.Boost = 0
				g.burst(riderX, 570)
				g.say("哎哟！丢了两条鱼，抬起车头再来。")
				if g.Hearts == 0 {
					g.end(crashed)
					return
				}
			}
		}
	}
	for i := range g.Particles {
		p := &g.Particles[i]
		if p.Life <= 0 {
			continue
		}
		p.Life = max(0, p.Life-dt)
		p.X += p.VX*dt - speed*dt*.3
		p.Y += p.VY * dt
		p.VY += 250 * dt
	}
	if g.Distance >= routeLength {
		g.Distance = routeLength
		g.end(finished)
	}
}
func (g *game) end(p phase) {
	g.Phase = p
	g.Held = false
	g.Gliding = false
	if p == finished {
		g.Score += g.Hearts * 150
		if g.Delivered >= target {
			g.Score += 1000
		}
	}
	g.Best = max(g.Best, g.Score)
}
