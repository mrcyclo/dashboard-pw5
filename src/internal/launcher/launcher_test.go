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
	pic, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("%T, want gray with transparency", img)
	}
	solid, clear := 0, 0
	for i := 0; i < len(pic.Pix); i += 4 {
		if pic.Pix[i] != pic.Pix[i+1] || pic.Pix[i] != pic.Pix[i+2] {
			t.Fatal("the Kindle shows grays only")
		}
		switch pic.Pix[i+3] {
		case 255:
			solid++
		case 0:
			clear++
		}
	}
	if solid < 96*96/20 || clear == 0 {
		t.Fatalf("%d solid and %d see-through pixels: the shape and its transparent margin are kept", solid, clear)
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
