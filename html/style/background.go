package style

func initialBackgroundLayer() BackgroundLayer {
	return BackgroundLayer{
		SizeX: Keyword("auto"), SizeY: Keyword("auto"),
		PosX: Number(0), PosY: Number(0),
		PosXOrigin: "left", PosYOrigin: "top",
		RepeatX: Keyword("repeat"), RepeatY: Keyword("repeat"),
		Origin: "padding-box", Clip: "border-box", Attachment: "scroll",
	}
}

// BackgroundLayerCount includes the primary layer, whose image may be none.
func (computed *ComputedStyle) BackgroundLayerCount() int {
	return 1 + len(computed.ExtraBackgroundLayers)
}

// BackgroundLayer returns a borrowed layer. Interned styles must not be mutated.
func (computed *ComputedStyle) BackgroundLayer(index int) *BackgroundLayer {
	if index == 0 {
		return &computed.Background
	}
	return &computed.ExtraBackgroundLayers[index-1]
}

func ensureBackgroundLayerCount(computed *ComputedStyle, count int) {
	for computed.BackgroundLayerCount() < count {
		computed.ExtraBackgroundLayers = append(computed.ExtraBackgroundLayers, initialBackgroundLayer())
	}
}
