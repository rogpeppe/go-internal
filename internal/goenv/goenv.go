// goenv is used by various testscript-related package to avoid
// invoking `go env` multiple times independently
// when a single execution is sufficient.
package goenv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
)

// Unmarshal JSON-unmarshals the result of `go env -json` into the value pointed
// to by dst.
func Unmarshal(dst any) error {
	data, err := goEnv()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("failed to unmarshal environment from go command out: %v\n%s", err, data)
	}
	return nil
}

var goEnv = sync.OnceValues(func() ([]byte, error) {
	var stdout, stderr bytes.Buffer
	// Note: we explicitly mention all the variables that
	// any of the callers of goenv might need to avoid
	// fetching any of the variables that are costly to calculate.
	cmd := exec.Command("go", "env", "-json",
		"GOCACHE",
		"GOMODCACHE",
		"GONOPROXY",
		"GOPROXY",
		"GOROOT",
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to determine environment from go command: %v\n%v", err, &stderr)
	}
	return stdout.Bytes(), nil
})
