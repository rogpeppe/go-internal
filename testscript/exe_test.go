// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testscript

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

const holdOpenReadyEnv = "TESTSCRIPT_HOLDOPEN_READY"

type testM struct {
	run func() int
}

func (m testM) Run() int {
	return m.run()
}

func holdOpen() {
	if err := os.WriteFile(os.Getenv(holdOpenReadyEnv), nil, 0o666); err != nil {
		os.Exit(2)
	}
	time.Sleep(750 * time.Millisecond)
}

func TestMainCleanupRetriesAccessDenied(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows executable sharing semantics are required")
	}

	ready := filepath.Join(t.TempDir(), "ready")
	oldReady, hadOldReady := os.LookupEnv(holdOpenReadyEnv)
	if err := os.Setenv(holdOpenReadyEnv, ready); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadOldReady {
			_ = os.Setenv(holdOpenReadyEnv, oldReady)
		} else {
			_ = os.Unsetenv(holdOpenReadyEnv)
		}
	})

	var cmd *exec.Cmd
	code := testingMRun(testM{run: func() int {
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
	}}, map[string]func(){"holdopen": holdOpen})
	if cmd != nil {
		if err := cmd.Wait(); err != nil {
			t.Errorf("holdopen: %v", err)
		}
	}
	if code != 0 {
		t.Fatalf("testingMRun returned %d", code)
	}
}
