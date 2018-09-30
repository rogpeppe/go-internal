// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testscript

import (
	"fmt"
	"os"
	"strconv"
	"testing"
)

func printArgs() {
	fmt.Printf("%q\n", os.Args)
}

func exitWithStatus() {
	n, _ := strconv.Atoi(os.Args[1])
	os.Exit(n)
}

func TestMain(m *testing.M) {
	RegisterCommand("printargs", printArgs)
	RegisterCommand("status", exitWithStatus)
	os.Exit(m.Run())
}

func TestSimple(t *testing.T) {
	// TODO set temp directory.
	Run(t, Params{
		Dir: "scripts",
	})
	// TODO check that the temp directory has been removed.
}
