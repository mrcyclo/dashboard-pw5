//go:build !linux

package main

import "os"

// grab only exists on the Kindle; elsewhere the command builds for vetting only.
func grab(*os.File) error {
	return nil
}
