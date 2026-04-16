// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package goproxytest

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/rogpeppe/go-internal/internal/goenv"
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
//
// If a file exists in the top level of the [GoModProxyDir] directory
// named "_enable_overlay", then instead of replacing the Go proxy entirely, the
// existing Go proxy is used to serve modules that are not present in
// the test script. In this case it uses the existing GOMODPROXY
// download cache to avoid network downloads if it can, but takes care
// not to pollute it with the localhost proxy server content.
func Setup(p *testscript.Params) {
	origSetup := p.Setup
	p.Setup = func(env *testscript.Env) error {
		if origSetup != nil {
			if err := origSetup(env); err != nil {
				return err
			}
		}
		proxyDir := filepath.Join(env.WorkDir, GoModProxyDir)
		info, err := os.Stat(proxyDir)
		if err != nil || !info.IsDir() {
			return nil
		}
		srv, err := NewServer(proxyDir, "")
		if err != nil {
			return fmt.Errorf("cannot start Go proxy: %v", err)
		}
		env.Defer(srv.Close)

		env.Vars = append(env.Vars, "GONOSUMDB=*")
		if info, err := os.Stat(filepath.Join(proxyDir, "_enable_overlay")); err == nil && info.Mode().IsRegular() {
			// Overlay is enabled: overlay rather than replacing the proxy.
			// NOTE: if you change the list of names here, make sure
			// to keep ../internal/goenv/goenv.go:/^var goEnv
			// up to date.
			var goEnv struct {
				GOMODCACHE string
				GOPROXY    string
				GONOPROXY  string
			}
			if err := goenv.Unmarshal(&goEnv); err != nil {
				return err
			}
			if goEnv.GOMODCACHE == "" || goEnv.GOPROXY == "" {
				// Shouldn't happen.
				return fmt.Errorf("missing GOMODCACHE or GOPROXY from go env output")
			}
			// Look for local test modules first, falling back to the
			// existing module download cache. See example under the
			// GOPROXY entry in https://go.dev/ref/mod#environment-variables
			env.Vars = append(env.Vars,
				fmt.Sprintf("GOPROXY=%s,%s,%s",
					srv.URL,
					uriFromPath(filepath.Join(goEnv.GOMODCACHE, "/cache/download")),
					goEnv.GOPROXY,
				),
				// Pass through the following variables on the grounds that
				// they should be used when fetching external modules.
				// Hopefully they will be immaterial because tests shouldn't
				// be relying on modules that match them, but it seems safer
				// to do this.
				"GONOPROXY="+goEnv.GONOPROXY,
			)
		} else {
			env.Vars = append(env.Vars, "GOPROXY="+srv.URL)
		}
		return nil
	}
}

// uriFromPath returns a file:// URI for the supplied file path.
// This code was adapted from cuelang.org/go/internal/golangorgx/gopls/protocol.URIFromPath
func uriFromPath(path string) string {
	if path == "" {
		return ""
	}
	if !isWindowsDrivePath(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	// Check the file path again, in case it became absolute.
	if isWindowsDrivePath(path) {
		path = "/" + strings.ToUpper(string(path[0])) + path[1:]
	}
	return (&url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}).String()
}

// isWindowsDrivePath returns true if the file path is of the form used by
// Windows. We check if the path begins with a drive letter, followed by a ":".
// For example: C:/x/y/z.
func isWindowsDrivePath(path string) bool {
	if len(path) < 3 {
		return false
	}
	return unicode.IsLetter(rune(path[0])) && path[1] == ':'
}
