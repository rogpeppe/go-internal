//go:build unix && !darwin
// +build unix,!darwin

package testscript

import "os"

// cloneFile creates to as a hard link to the from file.
func cloneFile(from, to string) error {
	return os.Link(from, to)
}
