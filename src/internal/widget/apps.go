package widget

import (
	"strings"

	"claudedash/internal/grid"
)

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
	// appNameLine is about how many characters of the name fit one line of a tile.
	appNameLine = 13
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

// appTile is the icon with the name below on up to two lines. In a one-row
// widget the second line does not fit and the Kindle leaves it out.
func appTile(a AppTile) Node {
	button := Button("app", appIconSize, AppActionPrefix+a.ID).Plain()
	if a.Icon != "" {
		button = node("button", Node{"src": "/apps/" + a.ID + ".png?v=" + a.Icon, "size": appIconSize, "action": AppActionPrefix + a.ID})
	}
	tile := []Node{button}
	for _, line := range nameLines(a.Name) {
		tile = append(tile, Text(line, appNameSize))
	}
	return Col(4, tile...).Align("center")
}

// nameLines breaks a name between words into at most two lines; the Kindle
// shortens a second line that is still too long.
func nameLines(name string) []string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return nil
	}
	first := words[0]
	rest := words[1:]
	for len(rest) > 0 && len(first)+1+len(rest[0]) <= appNameLine {
		first += " " + rest[0]
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return []string{first}
	}
	return []string{first, strings.Join(rest, " ")}
}
