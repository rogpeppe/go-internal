// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modfile

import (
	"bytes"
	"fmt"
	"testing"
)

var addRequireTests = []struct {
	in   string
	path string
	vers string
	out  string
}{
	{
		`
		module m
		require x.y/z v1.2.3
		`,
		"x.y/z", "v1.5.6",
		`
		module m
		require x.y/z v1.5.6
		`,
	},
	{
		`
		module m
		require x.y/z v1.2.3
		`,
		"x.y/w", "v1.5.6",
		`
		module m
		require (
			x.y/z v1.2.3
			x.y/w v1.5.6
		)
		`,
	},
	{
		`
		module m
		require x.y/z v1.2.3
		require x.y/q/v2 v2.3.4
		`,
		"x.y/w", "v1.5.6",
		`
		module m
		require x.y/z v1.2.3
		require (
			x.y/q/v2 v2.3.4
			x.y/w v1.5.6
		)
		`,
	},
}

func TestAddRequire(t *testing.T) {
	for i, tt := range addRequireTests {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			f, err := Parse("in", []byte(tt.in), nil)
			if err != nil {
				t.Fatal(err)
			}
			g, err := Parse("out", []byte(tt.out), nil)
			if err != nil {
				t.Fatal(err)
			}
			golden, err := g.Format()
			if err != nil {
				t.Fatal(err)
			}

			if err := f.AddRequire(tt.path, tt.vers); err != nil {
				t.Fatal(err)
			}
			out, err := f.Format()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, golden) {
				t.Errorf("have:\n%s\nwant:\n%s", out, golden)
			}
		})
	}
}

var addDropReplaceTests = []struct {
	in      string
	oldpath string
	oldvers string
	newpath string
	newvers string
	dropOld bool
	out     string
}{
	{
		`
		module m
		require x.y/z v1.2.3
		`,
		"x.y/z", "v1.2.3",
		"my.x.y/z", "v1.2.3",
		false,
		`
		module m
		require x.y/z v1.2.3
		replace x.y/z v1.2.3 => my.x.y/z v1.2.3
		`,
	},

	{
		`
		module m
		require x.y/z v1.2.3
		replace x.y/z => my.x.y/z v0.0.0-20190214113530-db6c41c15648
		`,
		"x.y/z", "",
		"my.x.y/z", "v1.2.3",
		true,
		`
		module m
		require x.y/z v1.2.3
		replace x.y/z => my.x.y/z v1.2.3
		`,
	},
	{
		`
		module m
		require x.y/z v1.2.3
		replace x.y/z => my.x.y/z v0.0.0-20190214113530-db6c41c15648
		`,
		"x.y/z", "",
		"", "", // empty newpath and newvers - drop only, no add
		true,
		`
		module m
		require x.y/z v1.2.3
		`,
	},
}

func TestAddDropReplace(t *testing.T) {
	for i, tt := range addDropReplaceTests {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			f, err := Parse("in", []byte(tt.in), nil)
			if err != nil {
				t.Fatal(err)
			}
			g, err := Parse("out", []byte(tt.out), nil)
			if err != nil {
				t.Fatal(err)
			}
			golden, err := g.Format()
			if err != nil {
				t.Fatal(err)
			}
			if tt.dropOld {
				if err := f.DropReplace(tt.oldpath, tt.oldvers); err != nil {
					t.Fatal(err)
				}
			}
			if tt.newpath != "" || tt.newvers != "" {
				if err := f.AddReplace(tt.oldpath, tt.oldvers, tt.newpath, tt.newvers); err != nil {
					t.Fatal(err)
				}
			}
			f.Cleanup()
			out, err := f.Format()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, golden) {
				t.Errorf("have:\n%s\nwant:\n%s", out, golden)
			}
		})
	}
}
