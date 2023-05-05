package testscript

import "golang.org/x/sys/unix"

// cloneFile clones the file from to the file to.
func cloneFile(from, to string) error {
	return unix.Clonefile(from, to, 0)
}
