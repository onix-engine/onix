package game

import (
	"fmt"
	"math"
	"strconv"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

type visual struct {
	entity    module.Entity
	transform scene.UITransform
	sprite    scene.Sprite2D
	visible   bool
}

func bind(c *module.Context, id string) (visual, error) {
	e, err := c.Element(id)
	if err != nil {
		return visual{}, err
	}
	t, ok := c.Get[scene.UITransform](e)
	if !ok {
		return visual{}, fmt.Errorf("pelican: %s has no transform", id)
	}
	s, _ := c.Get[scene.Sprite2D](e)
	v, _ := c.Get[scene.Visibility](e)
	return visual{entity: e, transform: t, sprite: s, visible: v.Visible}, nil
}
func (v *visual) show(c *module.Context, show bool) error {
	if show == v.visible {
		return nil
	}
	v.visible = show
	return c.Set(v.entity, scene.Visibility{Visible: show})
}
func (v *visual) pose(c *module.Context, x, y, r, sx, sy float32) error {
	v.transform.Translation = scene.Px2(x, y)
	v.transform.Rotation = r
	v.transform.Scale[0] = sx
	v.transform.Scale[1] = sy
	return c.Set(v.entity, v.transform)
}

type Mod struct {
	game
	delivery                                       visual
	coast, road                                    [2]visual
	rider, rear, front, crank, pouch, wing, shadow visual
	objects                                        [20]visual
	templates                                      [5]scene.Sprite2D
	particles                                      [12]visual
	windLines                                      [3]visual
	nodes                                          [len(nodeIDs)]module.HTMLNode
	keys                                           [9]bool
	pointerHeld, skipJumpClick                     bool
	lastPhase                                      phase
	hudStamp                                       [7]int
	lastNotice, lastWeather                        string
}

const (
	nCargo = iota
	nDelivered
	nHearts
	nScore
	nProgress
	nDistance
	nChapter
	nChapterNo
	nWeather
	nNotice
	nHint
	nVeil
	nLabel
	nTitle
	nDescription
	nRules
	nStart
	nRestart
	nBest
	nPause
	nDeliver
)

var nodeIDs = [...]string{"cargo", "delivered", "hearts", "score", "progress", "distance", "chapter", "chapter-number", "weather", "notice", "station-hint", "veil", "sheet-label", "sheet-title", "sheet-description", "rules", "start", "restart", "best", "pause", "deliver"}
var distances = func() [157]string {
	var a [157]string
	for i := range a {
		a[i] = strconv.Itoa(i/10) + "." + strconv.Itoa(i%10) + " / 15.6 km"
	}
	return a
}()
var progressStyles = func() [101]string {
	var a [101]string
	for i := range a {
		a[i] = "width: " + strconv.Itoa(i) + "%;"
	}
	return a
}()

func (m *Mod) Install(c *module.Context) error {
	for _, b := range []struct {
		id string
		v  *visual
	}{
		{"rider", &m.rider}, {"rear-wheel", &m.rear}, {"front-wheel", &m.front}, {"crank", &m.crank},
		{"pouch", &m.pouch}, {"wing", &m.wing}, {"shadow", &m.shadow}, {"delivery", &m.delivery},
	} {
		v, err := bind(c, b.id)
		if err != nil {
			return err
		}
		*b.v = v
	}
	groups := []struct {
		prefix string
		values []visual
	}{
		{"coast-", m.coast[:]}, {"road-", m.road[:]}, {"object-", m.objects[:]}, {"particle-", m.particles[:]}, {"wind-", m.windLines[:]},
	}
	for _, group := range groups {
		for i := range group.values {
			v, err := bind(c, group.prefix+strconv.Itoa(i))
			if err != nil {
				return err
			}
			group.values[i] = v
		}
	}
	for i := range m.templates {
		v, err := bind(c, "template-"+strconv.Itoa(i))
		if err != nil {
			return err
		}
		m.templates[i] = v.sprite
	}
	hud, err := c.Element("hud")
	if err != nil {
		return err
	}
	doc, err := module.HTML(c, hud)
	if err != nil {
		return err
	}
	for i, id := range nodeIDs {
		n, ok := doc.FindByID(id)
		if !ok {
			return fmt.Errorf("pelican: missing HTML node %s", id)
		}
		m.nodes[i] = n
	}
	m.reset()
	m.lastPhase = 255
	for i := range m.hudStamp {
		m.hudStamp[i] = -1
	}
	return m.render(c)
}

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	if message.Topic != module.InputTopic {
		return nil
	}
	e, ok := message.Value[module.Input]()
	if !ok {
		return module.ErrType
	}
	m.handleInput(e)
	return m.render(c)
}
func (m *Mod) newRun() { m.keys = [9]bool{}; m.pointerHeld = false; m.skipJumpClick = false; m.start() }
func keyIndex(key input.KeyCode) int {
	switch key {
	case input.KeySpace:
		return 0
	case input.KeyUp:
		return 1
	case input.KeyW:
		return 2
	case input.KeyX:
		return 3
	case input.KeyD:
		return 4
	case input.KeyP:
		return 5
	case input.KeyEscape:
		return 6
	case input.KeyR:
		return 7
	case input.KeyEnter:
		return 8
	}
	return -1
}
func (m *Mod) jumpHeld() bool { return m.keys[0] || m.keys[1] || m.keys[2] || m.pointerHeld }
func (m *Mod) handleInput(e module.Input) {
	if e.HTMLKind == htmlui.EventClick {
		switch e.HTMLID {
		case "start":
			if m.Phase == paused {
				m.togglePause()
			} else {
				m.newRun()
			}
		case "restart":
			m.newRun()
		case "pause":
			m.togglePause()
		case "deliver":
			m.deliver()
		case "jump":
			if m.skipJumpClick {
				m.skipJumpClick = false
				return
			}
			m.jump()
			m.release() // Semantic activation (keyboard or accessibility).
		}
		return
	}
	event := e.Event
	switch event.Kind {
	case scene.InputEventKeyDown, scene.InputEventKeyUp:
		k := keyIndex(event.Key)
		if k < 0 {
			return
		}
		down := event.Kind == scene.InputEventKeyDown
		if m.keys[k] == down {
			return
		}
		m.keys[k] = down
		if !down {
			if k < 3 {
				m.Held = m.jumpHeld()
			}
			return
		}
		switch k {
		case 0, 1, 2:
			if m.Phase == ready {
				m.newRun()
			} else {
				m.jump()
			}
		case 3, 4:
			m.deliver()
		case 5, 6:
			m.togglePause()
		case 7:
			m.newRun()
		case 8:
			if m.Phase == paused {
				m.togglePause()
			} else if m.Phase != riding {
				m.newRun()
			}
		}
		m.keys[k] = true
	case scene.InputEventPress:
		if m.Phase == riding && (e.HTMLID == "jump" || e.HTMLID == "" && event.Y > 155 && event.Y < 695) {
			m.pointerHeld = true
			m.skipJumpClick = e.HTMLID == "jump"
			m.jump()
		}
	case scene.InputEventRelease, scene.InputEventCancel:
		m.pointerHeld = false
		m.Held = m.jumpHeld()
		if event.Kind == scene.InputEventCancel {
			m.skipJumpClick = false
			m.keys = [9]bool{}
			m.release()
		}
	}
}
func (m *Mod) Update(c *module.Context, t module.Tick) error {
	if m.Phase != riding {
		return nil
	}
	m.step(t.Delta)
	return m.render(c)
}

func (m *Mod) render(c *module.Context) error {
	var err error
	pose := func(v *visual, x, y, r, sx, sy float32) {
		if err == nil {
			err = v.pose(c, x, y, r, sx, sy)
		}
	}
	show := func(v *visual, b bool) {
		if err == nil {
			err = v.show(c, b)
		}
	}
	for i := range m.coast {
		pose(&m.coast[i], float32(i)*1280-float32(math.Mod(float64(m.Distance*.18), 1280)), 270, 0, 1, 1)
		pose(&m.road[i], float32(i)*1280-float32(math.Mod(float64(m.Distance), 1280)), 600, 0, 1, 1)
	}
	x, y, scale := riderX, float32(606)-m.Height, float32(1)
	if m.Phase == ready {
		x = 905
		y = 604
		scale = 1.27
	}
	rotation := float32(0)
	if m.Height > 0 {
		rotation = -.09 - m.Velocity*.00015
	}
	pose(&m.rider, x, y, rotation, scale, scale)
	show(&m.rider, m.Invincible == 0 || int(m.Elapsed*14)%2 == 0)
	wheelAngle := m.Distance / 32
	m.rear.transform.Pivot = [2]float32{35, 35}
	// Wheels rotate about their centers, with authored top-left positions.
	pose(&m.rear, -83, -70, wheelAngle, 1, 1)
	m.front.transform.Pivot = [2]float32{35, 35}
	pose(&m.front, 42, -70, wheelAngle, 1, 1)
	m.crank.transform.Pivot = [2]float32{18, 18}
	pose(&m.crank, 12, -52, wheelAngle*.65, 1, 1)
	pose(&m.pouch, 69, -176, 0, 1, 1+float32(m.Cargo)*.065)
	show(&m.wing, m.Gliding || m.Height > 0 && m.Velocity > 180)
	pose(&m.wing, -64, -237, float32(math.Sin(float64(m.Elapsed*16)))*.09, 1, 1)
	pose(&m.shadow, x-90, 596, 0, max(.45, 1-m.Height/500), 1)
	used := 0
	for i := m.First; i < m.Count; i++ {
		o := &m.Objects[i]
		screenX := o.X - m.Distance
		if screenX > 1450 {
			break
		}
		if o.Used && o.Kind != station || screenX < -180 || m.Phase == ready {
			continue
		}
		if used == len(m.objects) {
			break
		}
		v := &m.objects[used]
		used++
		template := m.templates[o.Kind]
		if template.Texture != v.sprite.Texture {
			v.sprite = template
			if err == nil {
				err = c.Set(v.entity, template)
			}
		}
		show(v, true)
		objectY := float32(606) - o.Height - template.Height
		if o.Kind == fish {
			objectY = 606 - o.Height - template.Height/2 + float32(math.Sin(float64(m.Elapsed*4+o.X)))*4
		}
		pose(v, screenX-template.Width/2, objectY, 0, 1, 1)
	}
	for i := used; i < len(m.objects); i++ {
		show(&m.objects[i], false)
	}
	show(&m.delivery, m.DeliveryTime > 0)
	if m.DeliveryTime > 0 {
		t := 1 - m.DeliveryTime/.65
		x := (riderX+95)*(1-t) + (m.DeliveryX-m.Distance)*t
		y := m.DeliveryY*(1-t) + 460*t - float32(math.Sin(float64(t)*math.Pi))*100
		pose(&m.delivery, x, y, -t*5, 1, 1)
	}
	for i := range m.particles {
		p := &m.Particles[i]
		v := &m.particles[i]
		show(v, p.Life > 0)
		if p.Life > 0 {
			s := min(1, p.Life*3)
			pose(v, p.X, p.Y, p.Life*5, s, s)
		}
	}
	for i := range m.windLines {
		show(&m.windLines[i], m.Phase == riding && (m.Boost > 0 || m.wind() || m.Gliding))
		pose(&m.windLines[i], 1280-float32(math.Mod(float64(m.Elapsed*330+float32(i)*375), 1400)), 340+float32(i)*75, 0, 1, 1)
	}
	m.renderHUD()
	if err == nil {
		if host := c.Host(); host != nil && host.RequestRender != nil {
			host.RequestRender()
		}
	}
	return err
}
func (m *Mod) renderHUD() {
	n := &m.nodes
	values := [7]int{m.Cargo, m.Delivered, m.Hearts, m.Score, int(m.Distance / routeLength * 100), int(m.Distance / 100), min(2, int(m.Distance/5200))}
	for i := 0; i < 4; i++ {
		if values[i] != m.hudStamp[i] {
			n[i].SetText(strconv.Itoa(values[i]))
		}
	}
	if values[4] != m.hudStamp[4] {
		n[nProgress].SetAttribute("style", progressStyles[values[4]])
	}
	if values[5] != m.hudStamp[5] {
		n[nDistance].SetText(distances[values[5]])
	}
	if values[6] != m.hudStamp[6] {
		n[nChapter].SetText([3]string{"贝壳湾的早晨", "风车岬的逆风", "最后一束灯塔光"}[values[6]])
		n[nChapterNo].SetText([3]string{"COASTAL ROUTE / 01", "COASTAL ROUTE / 02", "COASTAL ROUTE / 03"}[values[6]])
	}
	m.hudStamp = values
	weather := "海风轻轻，鲜鱼正好。"
	if m.wind() {
		weather = "逆风来了，展开翅膀借风！"
	}
	if m.Boost > 0 {
		weather = "顺风冲刺！鲜鱼已签收。"
	}
	if m.Gliding {
		weather = "滑翔中 · 嘴袋越轻，飞得越远。"
	}
	if weather != m.lastWeather {
		n[nWeather].SetText(weather)
		m.lastWeather = weather
	}
	notice := m.Notice
	if m.NoticeTime == 0 && m.Phase == riding {
		notice = "沿着鱼群起跳，靠近灯塔小站按 X 投递。"
	}
	if notice != m.lastNotice {
		n[nNotice].SetText(notice)
		m.lastNotice = notice
	}
	near := m.nearStation() >= 0 && m.Phase == riding
	n[nHint].SetHidden(!near)
	if near && m.Cargo > 0 {
		n[nDeliver].SetAttribute("class", "lit")
	} else {
		n[nDeliver].RemoveAttribute("class")
	}
	if m.Phase == m.lastPhase {
		return
	}
	m.lastPhase = m.Phase
	n[nVeil].SetHidden(m.Phase == riding)
	n[nRules].SetHidden(m.Phase != ready)
	n[nRestart].SetHidden(m.Phase == ready)
	n[nStart].SetHidden(m.Phase == crashed || m.Phase == finished)
	if m.Phase == ready {
		n[nVeil].RemoveAttribute("class")
	} else {
		n[nVeil].SetAttribute("class", "center")
	}
	n[nPause].SetText("暂停")
	switch m.Phase {
	case paused:
		n[nLabel].SetText("在海风里歇一会")
		n[nTitle].SetText("鲜鱼可以等一下。")
		n[nDescription].SetText("按 P / Esc 或点击继续，回到你的海岸路线。")
		n[nStart].SetText("继续骑行 →")
		n[nPause].SetText("继续")
	case finished, crashed:
		title, label := "安全抵达，鲜鱼送到！", "今日海岸邮差 · 任务完成"
		if m.Phase == crashed {
			title = "车歇菜了，海还在。"
			label = "今日送鲜暂告一段落"
		} else if m.Delivered < target {
			title = "到了！再多送一点。"
			label = "骑完全程 · 鲜鱼目标未达成"
		}
		n[nLabel].SetText(label)
		n[nTitle].SetText(title)
		n[nDescription].SetText("已投递 " + strconv.Itoa(m.Delivered) + " 条鱼 / 目标 18 条 · 成功送达 " + strconv.Itoa(m.Stops) + " 座小站。 本局积分 " + strconv.Itoa(m.Score) + "。")
		n[nBest].SetText("本次会话最佳 " + strconv.Itoa(m.Best) + " 分 · 满袋投递额外 +200 · 达成目标额外 +1000")
	}
}
