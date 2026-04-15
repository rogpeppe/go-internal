// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package goproxytest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rogpeppe/go-internal/testscript"
)

// GoModProxyDir is the name of the special subdirectory in a txtar script's
// supporting files within which module archives for goproxytest are expected
// to be found.
const GoModProxyDir = ".gomodproxy"

// Setup sets up the given test parameters so that test scripts that
// contain a [GoModProxyDir] directory will have GOPROXY set to
// serve modules from that directory using a localhost proxy server.
//
// It wraps p.Setup to start a proxy server when the directory is present,
// set GOPROXY and GONOSUMDB appropriately, and shuts down
// the server when the test completes.
func Setup(p *testscript.Params) {
	origSetup := p.Setup
	p.Setup = func(env *testscript.Env) error {
		if origSetup != nil {
			if err := origSetup(env); err != nil {
				return err
			}
		}
		proxyDir := filepath.Join(env.WorkDir, GoModProxyDir)
		if info, err := os.Stat(proxyDir); err == nil && info.IsDir() {
			srv, err := NewServer(proxyDir, "")
			if err != nil {
				return fmt.Errorf("cannot start Go proxy: %v", err)
			}
			env.Defer(srv.Close)
			// Add GOPROXY after calling the original setup
			// so that it overrides any GOPROXY set there.
			env.Vars = append(env.Vars,
				"GOPROXY="+srv.URL,
				"GONOSUMDB=*",
			)
		}
		return nil
	}
}
