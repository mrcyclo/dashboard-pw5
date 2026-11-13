package widget

import (
	"fmt"
	"testing"
)

func tiles(n int) []AppTile {
	var out []AppTile
	for i := range n {
		out = append(out, AppTile{ID: fmt.Sprintf("id%d", i), Name: fmt.Sprintf("App %d", i), Icon: fmt.Sprintf("h%d", i)})
	}
	return out
}

func TestAppsShowOneTilePerCell(t *testing.T) {
	cases := map[string]int{"3x1": 3, "6x1": 6, "3x2": 6, "6x2": 12}
	for size, cells := range cases {
		tree := appsKind{}.Build(size, Data{Apps: tiles(20)})
		buttons := byKind(tree, "button")
		if len(buttons) != cells {
			t.Fatalf("%s: %d tiles, want %d", size, len(buttons), cells)
		}
		first := buttons[0]
		if first["action"] != "apps.open.id0" || first["src"] != "/apps/id0.png?v=h0" {
			t.Fatalf("%s: %v", size, first)
		}
	}
	if got := texts(appsKind{}.Build("6x1", Data{Apps: tiles(2)})); len(got) != 2 || got[1] != "App 1" {
		t.Fatalf("names under the icons: %v", got)
	}
}

func TestAppsWithNoneAddedSaySo(t *testing.T) {
	tree := appsKind{}.Build("3x1", Data{})
	if len(byKind(tree, "button")) != 0 || len(texts(tree)) != 1 {
		t.Fatalf("%v", tree)
	}
}

func TestAppsAreInTheCatalog(t *testing.T) {
	if _, ok := Default(nil).Get("apps"); !ok {
		t.Fatal("missing")
	}
}
