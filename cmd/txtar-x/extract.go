// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The txtar-x command extracts a txtar archive to a filesystem.
//
// Usage:
//
//	txtar-x [-C root-dir] saved.txt
//
// See https://godoc.org/github.com/rogpeppe/go-internal/txtar for details of the format
// and how to parse a txtar file.
//
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/rogpeppe/go-internal/txtar"
)

var (
	extractDir = flag.String("C", ".", "directory to extract files into")
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: txtar-x [flags] [file]\n")
	flag.PrintDefaults()
}

func main() {
	os.Exit(main1())
}

func main1() int {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() > 1 {
		usage()
		return 2
	}
	log.SetPrefix("txtar-x: ")
	log.SetFlags(0)

	var a *txtar.Archive
	if flag.NArg() == 0 {
		data, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			log.Printf("cannot read stdin: %v", err)
			return 1
		}
		a = txtar.Parse(data)
	} else {
		a1, err := txtar.ParseFile(flag.Arg(0))
		if err != nil {
			log.Print(err)
			return 1
		}
		a = a1
	}
	if err := extract(a); err != nil {
		log.Print(err)
		return 1
	}
	return 0
}

func extract(a *txtar.Archive) error {
	for _, f := range a.Files {
		if err := extractFile(f); err != nil {
			return fmt.Errorf("cannot extract %q: %v", f.Name, err)
		}
	}
	return nil
}

func extractFile(f txtar.File) error {
	path := filepath.Clean(filepath.FromSlash(f.Name))
	if isAbs(path) || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return fmt.Errorf("outside parent directory")
	}
	path = filepath.Join(*extractDir, path)
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	// Avoid overwriting existing files by using O_EXCL.
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := out.Write(f.Data); err != nil {
		return err
	}
	return nil
}

func isAbs(p string) bool {
	// Note: under Windows, filepath.IsAbs(`\foo`) returns false,
	// so we need to check for that case specifically.
	return filepath.IsAbs(p) || strings.HasPrefix(p, string(filepath.Separator))
}
