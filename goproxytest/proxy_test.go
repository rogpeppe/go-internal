// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package goproxytest_test

import (
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/goproxytest"
	"github.com/rogpeppe/go-internal/gotooltest"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestScripts(t *testing.T) {
	srv := goproxytest.NewTestServer(t, filepath.Join("testdata", "mod"), "")
	p := testscript.Params{
		Dir: "testdata",
		Setup: func(e *testscript.Env) error {
			e.Vars = append(e.Vars,
				"GOPROXY="+srv.URL,
				"GONOSUMDB=*",
			)
			return nil
		},
	}
	if err := gotooltest.Setup(&p); err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, p)
}

func TestSetup(t *testing.T) {
	t.Run("with_proxy", func(t *testing.T) {
		p := testscript.Params{
			Files: []string{
				filepath.Join("testdata", "setup", "with_proxy.txt"),
			},
		}
		if err := gotooltest.Setup(&p); err != nil {
			t.Fatal(err)
		}
		goproxytest.Setup(&p)
		testscript.Run(t, p)
	})
	t.Run("no_proxy", func(t *testing.T) {
		p := testscript.Params{
			Files: []string{
				filepath.Join("testdata", "setup", "no_proxy.txt"),
			},
			Setup: func(e *testscript.Env) error {
				e.Vars = append(e.Vars, "GOPROXY=original")
				return nil
			},
		}
		if err := gotooltest.Setup(&p); err != nil {
			t.Fatal(err)
		}
		goproxytest.Setup(&p)
		testscript.Run(t, p)
	})
	t.Run("chained", func(t *testing.T) {
		p := testscript.Params{
			Files: []string{
				filepath.Join("testdata", "setup", "chained.txt"),
			},
			Setup: func(e *testscript.Env) error {
				e.Vars = append(e.Vars, "CUSTOM=hello")
				return nil
			},
		}
		if err := gotooltest.Setup(&p); err != nil {
			t.Fatal(err)
		}
		goproxytest.Setup(&p)
		testscript.Run(t, p)
	})
}
