package desk

import (
	"errors"
	"testing"
)

type fakeLauncher struct {
	opened  [][2]string
	openErr error
	icons   int
}

func (f *fakeLauncher) Icon(path string, size int) ([]byte, error) {
	f.icons++
	if path == `C:\broken.exe` {
		return nil, errors.New("no icon")
	}
	return []byte("png:" + path), nil
}

func (f *fakeLauncher) Target(path string) string { return `C:\target\` + path[len(path)-5:] }

func (f *fakeLauncher) DisplayName(path string) string { return "Name of " + path }

func (f *fakeLauncher) Open(path, target string) error {
	f.opened = append(f.opened, [2]string{path, target})
	return f.openErr
}

func TestAddApp(t *testing.T) {
	h := newHarness(t)
	h.pickApp = `C:\Apps\one.exe`
	s, err := h.app.AddApp()
	if err != nil || len(s.Widgets.Apps) != 1 {
		t.Fatalf("%+v %v", s.Widgets.Apps, err)
	}
	app := s.Widgets.Apps[0]
	if app.Name != `Name of C:\Apps\one.exe` || app.ID == "" || app.Icon == "" {
		t.Fatalf("%+v", app)
	}
	if saved := h.saved(t).Apps; len(saved) != 1 || saved[0].Path != `C:\Apps\one.exe` {
		t.Fatalf("not saved: %+v", saved)
	}
	if _, err := h.app.AddApp(); err == nil {
		t.Fatal("the same program twice")
	}
	h.pickApp = ""
	if s, err := h.app.AddApp(); err != nil || len(s.Widgets.Apps) != 1 {
		t.Fatal("a cancelled dialog adds nothing")
	}
}

func TestRemoveAndMoveApps(t *testing.T) {
	h := newHarness(t)
	var ids []string
	for _, path := range []string{`C:\a\1.exe`, `C:\a\2.exe`, `C:\a\3.exe`} {
		h.pickApp = path
		s, _ := h.app.AddApp()
		ids = append(ids, s.Widgets.Apps[len(s.Widgets.Apps)-1].ID)
	}
	s, err := h.app.MoveApp(ids[2], -1)
	if err != nil || s.Widgets.Apps[1].ID != ids[2] || s.Widgets.Apps[2].ID != ids[1] {
		t.Fatalf("%+v %v", s.Widgets.Apps, err)
	}
	if s, _ := h.app.MoveApp(ids[0], -1); s.Widgets.Apps[0].ID != ids[0] {
		t.Fatal("the first stays first")
	}
	s, err = h.app.RemoveApp(ids[0])
	if err != nil || len(s.Widgets.Apps) != 2 || len(h.saved(t).Apps) != 2 {
		t.Fatalf("%+v %v", s.Widgets.Apps, err)
	}
	if _, err := h.app.RemoveApp("nope"); err == nil {
		t.Fatal("unknown app")
	}
}

func TestOpenAppFromTheKindle(t *testing.T) {
	h := newHarness(t)
	h.pickApp = `C:\Apps\one.exe`
	s, _ := h.app.AddApp()
	id := s.Widgets.Apps[0].ID
	if err := h.app.Act("apps.open." + id); err != nil {
		t.Fatal(err)
	}
	if len(h.launcher.opened) != 1 || h.launcher.opened[0] != [2]string{`C:\Apps\one.exe`, `C:\target\e.exe`} {
		t.Fatalf("%v", h.launcher.opened)
	}
	if err := h.app.Act("apps.open.nope"); err == nil {
		t.Fatal("unknown app")
	}
}

func TestAppIconsAreServedAndKept(t *testing.T) {
	h := newHarness(t)
	h.pickApp = `C:\Apps\one.exe`
	s, _ := h.app.AddApp()
	app := s.Widgets.Apps[0]
	data, hash, ok := h.app.AppIcon(app.ID)
	if !ok || string(data) != `png:C:\Apps\one.exe` || hash != app.Icon {
		t.Fatalf("%q %q %v", data, hash, ok)
	}
	before := h.launcher.icons
	h.app.GetState()
	h.app.AppIcon(app.ID)
	if h.launcher.icons != before {
		t.Fatal("an icon is drawn once, not on every poll")
	}
	if _, _, ok := h.app.AppIcon("nope"); ok {
		t.Fatal("unknown app")
	}
}

func TestAppWithoutAnIconStillShows(t *testing.T) {
	h := newHarness(t)
	h.pickApp = `C:\broken.exe`
	s, err := h.app.AddApp()
	if err != nil || len(s.Widgets.Apps) != 1 || s.Widgets.Apps[0].Icon != "" {
		t.Fatalf("%+v %v", s.Widgets.Apps, err)
	}
}
