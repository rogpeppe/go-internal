//go:build !unix
// +build !unix

package testscript

import "fmt"

// We don't want to use hard links on Windows, as that can lead to "access denied" errors when removing.
func cloneFile(from, to string) error {
	return fmt.Errorf("unavailable")
}
