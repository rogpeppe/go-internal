// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modfile

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/rogpeppe/go-internal/internal/textutil"
)

// exists reports whether the named file exists.
func exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

func TestParseLax(t *testing.T) {
	badFile := []byte(`module m
		surprise attack
		x y (
			z
		)
		exclude v1.2.3
		replace <-!!!
	`)
	_, err := ParseLax("file", badFile, nil)
	if err != nil {
		t.Fatalf("ParseLax did not ignore irrelevant errors: %v", err)
	}
}

// An eqchecker holds state for checking the equality of two parse trees.
type eqchecker struct {
	file string
	pos  Position
}

// errorf returns an error described by the printf-style format and arguments,
// inserting the current file position before the error text.
func (eq *eqchecker) errorf(format string, args ...interface{}) error {
	return fmt.Errorf("%s:%d: %s", eq.file, eq.pos.Line,
		fmt.Sprintf(format, args...))
}

// check checks that v and w represent the same parse tree.
// If not, it returns an error describing the first difference.
func (eq *eqchecker) check(v, w interface{}) error {
	return eq.checkValue(reflect.ValueOf(v), reflect.ValueOf(w))
}

var (
	posType      = reflect.TypeOf(Position{})
	commentsType = reflect.TypeOf(Comments{})
)

// checkValue checks that v and w represent the same parse tree.
// If not, it returns an error describing the first difference.
func (eq *eqchecker) checkValue(v, w reflect.Value) error {
	// inner returns the innermost expression for v.
	// if v is a non-nil interface value, it returns the concrete
	// value in the interface.
	inner := func(v reflect.Value) reflect.Value {
		for {
			if v.Kind() == reflect.Interface && !v.IsNil() {
				v = v.Elem()
				continue
			}
			break
		}
		return v
	}

	v = inner(v)
	w = inner(w)
	if v.Kind() == reflect.Invalid && w.Kind() == reflect.Invalid {
		return nil
	}
	if v.Kind() == reflect.Invalid {
		return eq.errorf("nil interface became %s", w.Type())
	}
	if w.Kind() == reflect.Invalid {
		return eq.errorf("%s became nil interface", v.Type())
	}

	if v.Type() != w.Type() {
		return eq.errorf("%s became %s", v.Type(), w.Type())
	}

	if p, ok := v.Interface().(Expr); ok {
		eq.pos, _ = p.Span()
	}

	switch v.Kind() {
	default:
		return eq.errorf("unexpected type %s", v.Type())

	case reflect.Bool, reflect.Int, reflect.String:
		vi := v.Interface()
		wi := w.Interface()
		if vi != wi {
			return eq.errorf("%v became %v", vi, wi)
		}

	case reflect.Slice:
		vl := v.Len()
		wl := w.Len()
		for i := 0; i < vl || i < wl; i++ {
			if i >= vl {
				return eq.errorf("unexpected %s", w.Index(i).Type())
			}
			if i >= wl {
				return eq.errorf("missing %s", v.Index(i).Type())
			}
			if err := eq.checkValue(v.Index(i), w.Index(i)); err != nil {
				return err
			}
		}

	case reflect.Struct:
		// Fields in struct must match.
		t := v.Type()
		n := t.NumField()
		for i := 0; i < n; i++ {
			tf := t.Field(i)
			switch {
			default:
				if err := eq.checkValue(v.Field(i), w.Field(i)); err != nil {
					return err
				}

			case tf.Type == posType: // ignore positions
			case tf.Type == commentsType: // ignore comment assignment
			}
		}

	case reflect.Ptr, reflect.Interface:
		if v.IsNil() != w.IsNil() {
			if v.IsNil() {
				return eq.errorf("unexpected %s", w.Elem().Type())
			}
			return eq.errorf("missing %s", v.Elem().Type())
		}
		if err := eq.checkValue(v.Elem(), w.Elem()); err != nil {
			return err
		}
	}
	return nil
}

// tdiff logs the diff output to t.Error.
func tdiff(t *testing.T, a, b string) {
	data := textutil.Diff(a, b)
	t.Error(string(data))
}

var modulePathTests = []struct {
	input    []byte
	expected string
}{
	{input: []byte("module \"github.com/rsc/vgotest\""), expected: "github.com/rsc/vgotest"},
	{input: []byte("module github.com/rsc/vgotest"), expected: "github.com/rsc/vgotest"},
	{input: []byte("module  \"github.com/rsc/vgotest\""), expected: "github.com/rsc/vgotest"},
	{input: []byte("module  github.com/rsc/vgotest"), expected: "github.com/rsc/vgotest"},
	{input: []byte("module `github.com/rsc/vgotest`"), expected: "github.com/rsc/vgotest"},
	{input: []byte("module \"github.com/rsc/vgotest/v2\""), expected: "github.com/rsc/vgotest/v2"},
	{input: []byte("module github.com/rsc/vgotest/v2"), expected: "github.com/rsc/vgotest/v2"},
	{input: []byte("module \"gopkg.in/yaml.v2\""), expected: "gopkg.in/yaml.v2"},
	{input: []byte("module gopkg.in/yaml.v2"), expected: "gopkg.in/yaml.v2"},
	{input: []byte("module \"gopkg.in/check.v1\"\n"), expected: "gopkg.in/check.v1"},
	{input: []byte("module \"gopkg.in/check.v1\n\""), expected: ""},
	{input: []byte("module gopkg.in/check.v1\n"), expected: "gopkg.in/check.v1"},
	{input: []byte("module \"gopkg.in/check.v1\"\r\n"), expected: "gopkg.in/check.v1"},
	{input: []byte("module gopkg.in/check.v1\r\n"), expected: "gopkg.in/check.v1"},
	{input: []byte("module \"gopkg.in/check.v1\"\n\n"), expected: "gopkg.in/check.v1"},
	{input: []byte("module gopkg.in/check.v1\n\n"), expected: "gopkg.in/check.v1"},
	{input: []byte("module \n\"gopkg.in/check.v1\"\n\n"), expected: ""},
	{input: []byte("module \ngopkg.in/check.v1\n\n"), expected: ""},
	{input: []byte("module \"gopkg.in/check.v1\"asd"), expected: ""},
	{input: []byte("module \n\"gopkg.in/check.v1\"\n\n"), expected: ""},
	{input: []byte("module \ngopkg.in/check.v1\n\n"), expected: ""},
	{input: []byte("module \"gopkg.in/check.v1\"asd"), expected: ""},
	{input: []byte("module  \nmodule a/b/c "), expected: "a/b/c"},
	{input: []byte("module \"   \""), expected: "   "},
	{input: []byte("module   "), expected: ""},
	{input: []byte("module \"  a/b/c  \""), expected: "  a/b/c  "},
	{input: []byte("module \"github.com/rsc/vgotest1\" // with a comment"), expected: "github.com/rsc/vgotest1"},
}

func TestModulePath(t *testing.T) {
	for _, test := range modulePathTests {
		t.Run(string(test.input), func(t *testing.T) {
			result := ModulePath(test.input)
			if result != test.expected {
				t.Fatalf("ModulePath(%q): %s, want %s", string(test.input), result, test.expected)
			}
		})
	}
}
