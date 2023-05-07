This repository factors out an opinionated selection of internal packages and functionality from the Go standard
library. Currently this consists mostly of packages and testing code from within the Go tool implementation.

This repo is [primarily maintained](https://github.com/rogpeppe/go-internal/graphs/contributors) by long-time [Go contributors](https://github.com/golang/go/contributors) who are also currently [maintaining CUE](https://github.com/cue-lang/cue/graphs/contributors) (which is primarily written in Go and which relies upon several of the packages here).

Contributions are welcome, but please open an issue for discussion first.

### Packages

Included are the following:

- dirhash: calculate hashes over directory trees the same way that the Go tool does.
- goproxytest: a GOPROXY implementation designed for test use.
- gotooltest: Use the Go tool inside test scripts (see testscript below)
- imports: list of known architectures and OSs, and support for reading import statements.
- modfile: read and write `go.mod` files while preserving formatting and comments.
- module: module paths and versions.
- par: do work in parallel.
- semver: semantic version parsing.
- testenv: information on the current testing environment.
- testscript: script-based testing based on txtar files
- txtar: simple text-based file archives for testing.

### testscript

The most popular package here is the [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) package:
 * Provides a shell-like test environment that is very nicely tuned for testing Go CLI commands.
 * Extracted version of the core Go team's internal testscript package.
 * [Heavily used](https://github.com/golang/go/tree/master/src/cmd/go/testdata/script) to test the `go` command.
 * Includes patterns for checking stderr/stdout, command pass/fail assertions, and so on.
 * Built-in support for Go concepts like build tags.
 * Integrates well with `go test`, including coverage support.
 * Inputs and sample outputs are in the [txtar](https://pkg.go.dev/golang.org/x/tools/txtar) text archive format, also used by the Go playground.
 * Accompanied by a [testscript](https://github.com/rogpeppe/go-internal/tree/master/cmd/testscript) command for running standalone scripts with files embedded in txtar format.
 
 A nice introduction to using testscripts is this [blog post](https://bitfieldconsulting.com/golang/test-scripts) series.
