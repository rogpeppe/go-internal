package testenv

import (
	"os"
	"path/filepath"
	"runtime"
)

// HasExec reports whether the current system can start new processes
// using os.StartProcess or (more commonly) exec.Command.
func HasExec() bool {
	return tryExec() == nil
}

// HasSrc reports whether the entire source tree is available under GOROOT.
func HasSrc() bool {
	switch runtime.GOOS {
	case "ios":
		return false
	}
	if Builder() != "" {
		// The builders have the source tree available, and if they don't the
		// tests should error out.
		return true
	}
	// If not running on the builders, the test binary might have been copied to
	// a target machine where the source tree isn't available.
	goroot, err := findGOROOT()
	if err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(goroot, "src", "go.mod")); err != nil {
		return false
	}
	return true
}
