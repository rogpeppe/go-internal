// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package renameio is a thin wrapper over [github.com/google/renameio/v2].
// See that package for full documentation.
package renameio

import (
	"io"
	"path/filepath"

	renameio "github.com/google/renameio/v2"
)

const patternSuffix = "*.tmp"

// Pattern originally returned a glob pattern that matched the unrenamed temporary files
// created when writing to filename. Now it doesn't. It was always prone to false
// positives in any case.
//
// Deprecated: this does not work.
func Pattern(filename string) string {
	return filepath.Join(filepath.Dir(filename), filepath.Base(filename)+patternSuffix)
}

// WriteFile is like ioutil.WriteFile, but first writes data to an arbitrary
// file in the same directory as filename, then renames it atomically to the
// final name.
//
// That ensures that the final location, if it exists, is always a complete file.
func WriteFile(filename string, data []byte) (err error) {
	return renameio.WriteFile(filename, data, 0o666)
}

// WriteToFile is a variant of WriteFile that accepts the data as an io.Reader
// instead of a slice.
func WriteToFile(filename string, data io.Reader) (err error) {
	f, err := renameio.NewPendingFile(filename)
	if err != nil {
		return err
	}
	defer f.Cleanup()
	if _, err := io.Copy(f, data); err != nil {
		return err
	}
	return f.CloseAtomicallyReplace()
}
