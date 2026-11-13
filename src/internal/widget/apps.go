package widget

import "claudedash/internal/grid"

// AppTile is one program the apps widget shows. Icon is the hash of its
// picture, served at /apps/<id>.png, or "" when it has none.
type AppTile struct {
	ID   string
	Name string
	Icon string
}

// AppActionPrefix starts the action of an app tile; the app's id follows.
const AppActionPrefix = "apps.open."

const (
	appIconSize = 88
	appNameSize = 22
)

type appsKind struct{}

func (appsKind) Name() string    { return "apps" }
func (appsKind) Label() string   { return "Apps" }
func (appsKind) Icon() string    { return "app-window" }
func (appsKind) Sizes() []string { return []string{"3x1", "6x1", "3x2", "6x2"} }

// Build gives each grid cell one app, in the order of the list; apps past the last cell are left out.
func (appsKind) Build(size string, d Data) Node {
	if len(d.Apps) == 0 {
		return Col(0, Text("Add apps on the PC, on the Layout page", 26).Dim()).Align("center")
	}
	s, _ := grid.ParseSize(size)
	rows := []Node{}
	for r := range s.Rows {
		cells := []Node{}
		for c := range s.Cols {
			i := r*s.Cols + c
			if i >= len(d.Apps) {
				cells = append(cells, Space(0).Grow(1))
				continue
			}
			cells = append(cells, appTile(d.Apps[i]).Grow(1))
		}
		rows = append(rows, Row(0, cells...).Grow(1))
	}
	return Col(0, rows...)
}

func appTile(a AppTile) Node {
	button := Button("app", appIconSize, AppActionPrefix+a.ID).Plain()
	if a.Icon != "" {
		button = node("button", Node{"src": "/apps/" + a.ID + ".png?v=" + a.Icon, "size": appIconSize, "action": AppActionPrefix + a.ID})
	}
	return Col(6, button, Text(a.Name, appNameSize)).Align("center")
}
