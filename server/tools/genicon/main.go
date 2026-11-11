// Command genicon writes the application icon files the Windows build needs.
package main

import (
	"log"
	"os"
	"path/filepath"

	"claudedash/internal/tray"
)

func main() {
	files := map[string][]byte{
		filepath.Join("build", "windows", "icon.ico"): tray.Icon(),
		filepath.Join("build", "appicon.png"):         tray.IconPNG(512),
	}
	for path, data := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
