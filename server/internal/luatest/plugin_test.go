package luatest

import (
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

const pluginDir = "../../../claudedash.koplugin"

func runSpec(t *testing.T, name string) {
	t.Helper()
	dir, err := filepath.Abs(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	L := lua.NewState()
	defer L.Close()
	searchPath := strings.ReplaceAll(filepath.Join(dir, "?.lua"), `\`, "/")
	L.SetField(L.GetGlobal("package"), "path", lua.LString(searchPath))
	if err := L.DoFile(filepath.Join(dir, "spec", name)); err != nil {
		t.Fatal(err)
	}
}

func TestFormatSpec(t *testing.T) {
	runSpec(t, "format_spec.lua")
}

func TestLayoutSpec(t *testing.T) {
	runSpec(t, "layout_spec.lua")
}

func TestAsyncSpec(t *testing.T) {
	runSpec(t, "async_spec.lua")
}

func TestPluginFilesCompile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(pluginDir, "*.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 6 {
		t.Fatalf("expected the six plugin files, found %d", len(files))
	}
	L := lua.NewState()
	defer L.Close()
	for _, file := range files {
		if _, err := L.LoadFile(file); err != nil {
			t.Errorf("%s: %v", filepath.Base(file), err)
		}
	}
}
