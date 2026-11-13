package main

import (
	"os"
	"testing"
)

func TestCoverIsAPortraitPage(t *testing.T) {
	data, err := os.ReadFile("../../../kindle/package/assets/NotoSans-Bold.ttf")
	if err != nil {
		t.Fatal(err)
	}
	img, err := cover(data)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dy() <= b.Dx() {
		t.Fatalf("a book cover is taller than wide: %v", b)
	}
	// The gauge's center dot sits just below the middle of the icon.
	if c := img.NRGBAAt(coverWidth/2, iconTop+iconSize*52/100); c == background {
		t.Fatal("the icon is drawn in the middle")
	}
}
