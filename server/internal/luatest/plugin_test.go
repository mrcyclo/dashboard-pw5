package luatest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

const pluginDir = "../../../claudedash.koplugin"

// toLua turns decoded JSON into Lua values, so specs can read the shared fixtures.
func toLua(L *lua.LState, v any) lua.LValue {
	switch value := v.(type) {
	case map[string]any:
		table := L.NewTable()
		for key, item := range value {
			table.RawSetString(key, toLua(L, item))
		}
		return table
	case []any:
		table := L.NewTable()
		for _, item := range value {
			table.Append(toLua(L, item))
		}
		return table
	case float64:
		return lua.LNumber(value)
	case string:
		return lua.LString(value)
	case bool:
		return lua.LBool(value)
	}
	return lua.LNil
}

func runSpec(t *testing.T, name string, globals ...func(*lua.LState)) {
	t.Helper()
	dir, err := filepath.Abs(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	L := lua.NewState()
	defer L.Close()
	searchPath := strings.ReplaceAll(filepath.Join(dir, "?.lua"), `\`, "/")
	L.SetField(L.GetGlobal("package"), "path", lua.LString(searchPath))
	for _, set := range globals {
		set(L)
	}
	if err := L.DoFile(filepath.Join(dir, "spec", name)); err != nil {
		t.Fatal(err)
	}
}

func TestFormatSpec(t *testing.T) {
	runSpec(t, "format_spec.lua")
}

func TestTreeSpec(t *testing.T) {
	data, err := os.ReadFile("../widget/testdata/tree_fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures any
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	runSpec(t, "tree_spec.lua", func(L *lua.LState) {
		L.SetGlobal("FIXTURES", toLua(L, fixtures))
	})
}

func TestPatchSpec(t *testing.T) {
	runSpec(t, "patch_spec.lua")
}

func TestViewSpec(t *testing.T) {
	runSpec(t, "view_spec.lua")
}

func TestScreenSpec(t *testing.T) {
	runSpec(t, "screen_spec.lua")
}

func TestAsyncSpec(t *testing.T) {
	runSpec(t, "async_spec.lua")
}

func TestPluginFilesCompile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(pluginDir, "*.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 10 {
		t.Fatalf("expected the ten plugin files, found %d", len(files))
	}
	L := lua.NewState()
	defer L.Close()
	for _, file := range files {
		if _, err := L.LoadFile(file); err != nil {
			t.Errorf("%s: %v", filepath.Base(file), err)
		}
	}
}
