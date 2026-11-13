package widget

type clockKind struct{}

func (clockKind) Name() string    { return "clock" }
func (clockKind) Label() string   { return "Clock" }
func (clockKind) Icon() string    { return "clock" }
func (clockKind) Sizes() []string { return []string{"3x1", "6x1"} }

// The Kindle fills in the time and date itself, so the clock keeps running with the PC off.
func (clockKind) Build(size string, _ Data) Node {
	if size == "6x1" {
		return Row(28, Clock("time", 96).Bold(), Clock("date", 35))
	}
	return Col(0, Clock("time", 72).Bold(), Clock("date", 29))
}
