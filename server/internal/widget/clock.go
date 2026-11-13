package widget

type clockKind struct{}

func (clockKind) Name() string    { return "clock" }
func (clockKind) Label() string   { return "Clock" }
func (clockKind) Icon() string    { return "clock" }
func (clockKind) Sizes() []string { return []string{"3x1", "3x2"} }

func (clockKind) Options() []Option {
	return []Option{{
		Key:   "align",
		Label: "Alignment",
		Choices: []Choice{
			{Value: "left", Label: "Left"},
			{Value: "center", Label: "Center"},
			{Value: "right", Label: "Right"},
		},
		Default: "left",
	}}
}

// alignments maps the align option to how a column places its children.
var alignments = map[string]string{"left": "start", "center": "center", "right": "end"}

// The Kindle fills in the time and date itself, so the clock keeps running with the PC off.
func (clockKind) Build(size string, d Data) Node {
	align, ok := alignments[d.Option("clock", "align", "left")]
	if !ok {
		align = "start"
	}
	if size == "3x2" {
		// Twice the height: much larger text, centered vertically.
		return Col(0, Space(0).Grow(1), Clock("time", 170).Bold(), Clock("date", 42), Space(0).Grow(1)).Align(align)
	}
	return Col(0, Clock("time", 72).Bold(), Clock("date", 29)).Align(align)
}
