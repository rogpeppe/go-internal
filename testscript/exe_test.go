// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testscript

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const holdOpenReadyEnv = "TESTSCRIPT_HOLDOPEN_READY"

type testM func() int

func (m testM) Run() int {
	return m()
}

func TestMainCleanupRetriesAccessDenied(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv(holdOpenReadyEnv, ready)

	var cmd *exec.Cmd
	code := testingMRun(testM(func() int {
		cmd = exec.Command("holdopen")
		if err := cmd.Start(); err != nil {
			t.Errorf("start holdopen: %v", err)
			return 1
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(ready); err == nil {
				return 0
			}
			if time.Now().After(deadline) {
				t.Error("holdopen did not signal readiness")
				return 1
			}
			time.Sleep(10 * time.Millisecond)
		}
	}), map[string]func(){"holdopen": holdOpen})
	if cmd != nil {
		if err := cmd.Wait(); err != nil {
			t.Errorf("holdopen: %v", err)
		}
	}
	if code != 0 {
		t.Fatalf("testingMRun returned %d", code)
	}
}

func holdOpen() {
	if err := os.WriteFile(os.Getenv(holdOpenReadyEnv), nil, 0o666); err != nil {
		os.Exit(2)
	}
	time.Sleep(100 * time.Millisecond)
}
