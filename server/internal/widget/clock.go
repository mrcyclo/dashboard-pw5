package widget

type clockKind struct{}

func (clockKind) Name() string    { return "clock" }
func (clockKind) Label() string   { return "Clock" }
func (clockKind) Icon() string    { return "clock" }
func (clockKind) Sizes() []string { return []string{"3x1", "3x2"} }

// The Kindle fills in the time and date itself, so the clock keeps running with the PC off.
func (clockKind) Build(size string, _ Data) Node {
	if size == "3x2" {
		// Twice the height: larger text, centered vertically.
		return Col(0, Space(0).Grow(1), Clock("time", 120).Bold(), Clock("date", 38), Space(0).Grow(1))
	}
	return Col(0, Clock("time", 72).Bold(), Clock("date", 29))
}
