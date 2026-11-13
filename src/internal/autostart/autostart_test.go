package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func useTestValue(t *testing.T) {
	t.Helper()
	old := valueName
	valueName = "ClaudeDashTest"
	t.Cleanup(func() {
		Disable()
		valueName = old
	})
}

func TestEnableDisableRoundTrip(t *testing.T) {
	useTestValue(t)
	if _, on, err := Command(); err != nil || on {
		t.Fatalf("before: %v %v", on, err)
	}
	if err := Enable(`C:\a b\x.exe`); err != nil {
		t.Fatal(err)
	}
	if _, on, err := Command(); err != nil || !on {
		t.Fatalf("after enable: %v %v", on, err)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	got, _, err := k.GetStringValue(valueName)
	if err != nil || got != `"C:\a b\x.exe"` {
		t.Fatalf("value %q err %v", got, err)
	}
	if err := Disable(); err != nil {
		t.Fatal(err)
	}
	if _, on, err := Command(); err != nil || on {
		t.Fatalf("after disable: %v %v", on, err)
	}
}

func TestDisableWhenAbsentIsNoError(t *testing.T) {
	useTestValue(t)
	if err := Disable(); err != nil {
		t.Fatal(err)
	}
}

func TestEnableWithArguments(t *testing.T) {
	useTestValue(t)
	if err := Enable(`C:\a b\x.exe`, "--hidden"); err != nil {
		t.Fatal(err)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	got, _, err := k.GetStringValue(valueName)
	if err != nil || got != `"C:\a b\x.exe" --hidden` {
		t.Fatalf("value %q err %v", got, err)
	}
}

func TestCommand(t *testing.T) {
	useTestValue(t)
	if _, ok, err := Command(); err != nil || ok {
		t.Fatalf("before: ok %v err %v", ok, err)
	}
	Enable(`C:\a b\x.exe`, "--hidden")
	command, ok, err := Command()
	if err != nil || !ok || command != CommandLine(`C:\a b\x.exe`, "--hidden") {
		t.Fatalf("got %q ok %v err %v", command, ok, err)
	}
}
