//go:build windows

package launcher

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const notepad = `C:\Windows\System32\notepad.exe`

func TestIconIsASquareGrayPicture(t *testing.T) {
	data, err := Icon(notepad, 96)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 96, 96) {
		t.Fatalf("bounds %v", img.Bounds())
	}
	gray, ok := img.(*image.Gray)
	if !ok {
		t.Fatalf("%T, want a gray picture for the Kindle", img)
	}
	dark := 0
	for _, v := range gray.Pix {
		if v < 200 {
			dark++
		}
	}
	if dark < 96*96/20 {
		t.Fatalf("only %d dark pixels: the icon is missing", dark)
	}
}

func TestIconOfAMissingFile(t *testing.T) {
	if _, err := Icon(filepath.Join(t.TempDir(), "nothing.exe"), 96); err == nil {
		t.Fatal("a file that does not exist has no icon")
	}
}

func TestTargetFollowsAShortcut(t *testing.T) {
	link := filepath.Join(t.TempDir(), "Notepad.lnk")
	if err := makeShortcut(link, notepad); err != nil {
		t.Fatal(err)
	}
	if got := Target(link); !strings.EqualFold(got, notepad) {
		t.Fatalf("got %q", got)
	}
	if got := Target(notepad); !strings.EqualFold(got, notepad) {
		t.Fatalf("a program is its own target: %q", got)
	}
	if got := Target(filepath.Join(t.TempDir(), "notes.txt")); got != "" {
		t.Fatalf("a document has no program to look for: %q", got)
	}
	os.Remove(link)
}

func TestSameProgramIgnoresCaseAndSlashes(t *testing.T) {
	if !sameProgram(`c:/windows/system32/NOTEPAD.EXE`, notepad) || sameProgram(`C:\Windows\notepad.exe`, notepad) || sameProgram("", notepad) {
		t.Fatal("paths are compared like Windows does")
	}
}

func TestDisplayNameUsesTheProgramDescription(t *testing.T) {
	if got := DisplayName(notepad); got != "Notepad" {
		t.Fatalf("got %q", got)
	}
	if got := DisplayName(filepath.Join(t.TempDir(), "My Tool.lnk")); got != "My Tool" {
		t.Fatalf("without a description the file name is used: %q", got)
	}
}
