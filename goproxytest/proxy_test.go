// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package goproxytest_test

import (
	"encoding/json"
	"io"
	"net/http"
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
				"testdata/setup/with_proxy.txt",
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
				"testdata/setup/no_proxy.txt",
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
				"testdata/setup/chained.txt",
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
	t.Run("overlay", func(t *testing.T) {
		p := testscript.Params{
			Files: []string{
				"testdata/setup/overlay.txt",
			},
		}
		if err := gotooltest.Setup(&p); err != nil {
			t.Fatal(err)
		}
		goproxytest.Setup(&p)
		testscript.Run(t, p)
	})
}

func TestLatest(t *testing.T) {
	srv := goproxytest.NewTestServer(t, filepath.Join("testdata", "mod"), "")

	tests := []struct {
		name   string
		module string
		want   string
	}{
		{
			name:   "release preferred over prerelease",
			module: "fruit.com",
			want:   "v1.1.0",
		},
		{
			name:   "prerelease only",
			module: "prerelease.example",
			want:   "v0.2.0-rc.1",
		},
		{
			name:   "unknown module",
			module: "noexist.example",
			want:   "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/" + test.module + "/@latest")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if test.want == "" {
				if resp.StatusCode != 404 {
					t.Fatalf("got status %d, want 404", resp.StatusCode)
				}
				return
			}
			if resp.StatusCode != 200 {
				t.Fatalf("got status %d, want 200", resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			var info struct {
				Version string
			}
			if err := json.Unmarshal(body, &info); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}
			if info.Version != test.want {
				t.Fatalf("got version %q, want %q", info.Version, test.want)
			}
		})
	}
}
