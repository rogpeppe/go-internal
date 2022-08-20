// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testscript

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func printArgs() int {
	fmt.Printf("%q\n", os.Args)
	return 0
}

func fprintArgs() int {
	s := strings.Join(os.Args[2:], " ")
	switch os.Args[1] {
	case "stdout":
		fmt.Println(s)
	case "stderr":
		fmt.Fprintln(os.Stderr, s)
	}
	return 0
}

func exitWithStatus() int {
	n, _ := strconv.Atoi(os.Args[1])
	return n
}

func signalCatcher() int {
	// Note: won't work under Windows.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	// Create a file so that the test can know that
	// we will catch the signal.
	if err := ioutil.WriteFile("catchsignal", nil, 0o666); err != nil {
		fmt.Println(err)
		return 1
	}
	<-c
	fmt.Println("caught interrupt")
	return 0
}

func TestMain(m *testing.M) {
	os.Exit(RunMain(m, map[string]func() int{
		"printargs":     printArgs,
		"fprintargs":    fprintArgs,
		"status":        exitWithStatus,
		"signalcatcher": signalCatcher,
	}))
}

func TestCRLFInput(t *testing.T) {
	td, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create TempDir: %v", err)
	}
	defer func() {
		os.RemoveAll(td)
	}()
	tf := filepath.Join(td, "script.txt")
	contents := []byte("exists output.txt\r\n-- output.txt --\r\noutput contents")
	if err := ioutil.WriteFile(tf, contents, 0o644); err != nil {
		t.Fatalf("failed to write to %v: %v", tf, err)
	}
	t.Run("_", func(t *testing.T) {
		Run(t, Params{Dir: td})
	})
}

func TestEnv(t *testing.T) {
	e := &Env{
		Vars: []string{
			"HOME=/no-home",
			"PATH=/usr/bin",
			"PATH=/usr/bin:/usr/local/bin",
			"INVALID",
		},
	}

	if got, want := e.Getenv("HOME"), "/no-home"; got != want {
		t.Errorf("e.Getenv(\"HOME\") == %q, want %q", got, want)
	}

	e.Setenv("HOME", "/home/user")
	if got, want := e.Getenv("HOME"), "/home/user"; got != want {
		t.Errorf(`e.Getenv("HOME") == %q, want %q`, got, want)
	}

	if got, want := e.Getenv("PATH"), "/usr/bin:/usr/local/bin"; got != want {
		t.Errorf(`e.Getenv("PATH") == %q, want %q`, got, want)
	}

	if got, want := e.Getenv("INVALID"), ""; got != want {
		t.Errorf(`e.Getenv("INVALID") == %q, want %q`, got, want)
	}

	for _, key := range []string{
		"",
		"=",
		"key=invalid",
	} {
		var panicValue interface{}
		func() {
			defer func() {
				panicValue = recover()
			}()
			e.Setenv(key, "")
		}()
		if panicValue == nil {
			t.Errorf("e.Setenv(%q) did not panic, want panic", key)
		}
	}
}

func TestScripts(t *testing.T) {
	// TODO set temp directory.
	testDeferCount := 0
	Run(t, Params{
		UpdateScripts: os.Getenv("TESTSCRIPT_UPDATE") != "",
		Dir:           "testdata",
		Cmds: map[string]func(ts *TestScript, neg bool, args []string){
			"setSpecialVal":    setSpecialVal,
			"ensureSpecialVal": ensureSpecialVal,
			"interrupt":        interrupt,
			"waitfile":         waitFile,
			"testdefer": func(ts *TestScript, neg bool, args []string) {
				testDeferCount++
				n := testDeferCount
				ts.Defer(func() {
					if testDeferCount != n {
						t.Errorf("defers not run in reverse order; got %d want %d", testDeferCount, n)
					}
					testDeferCount--
				})
			},
			"setup-filenames": func(ts *TestScript, neg bool, want []string) {
				got := ts.Value("setupFilenames")
				if !reflect.DeepEqual(want, got) {
					ts.Fatalf("setup did not see expected files; got %q want %q", got, want)
				}
			},
			"test-values": func(ts *TestScript, neg bool, args []string) {
				if ts.Value("somekey") != 1234 {
					ts.Fatalf("test-values did not see expected value")
				}
				if ts.Value("t").(T) != ts.t {
					ts.Fatalf("test-values did not see expected t")
				}
				if _, ok := ts.Value("t").(testing.TB); !ok {
					ts.Fatalf("test-values t does not implement testing.TB")
				}
			},
			"testreadfile": func(ts *TestScript, neg bool, args []string) {
				if len(args) != 1 {
					ts.Fatalf("testreadfile <filename>")
				}
				got := ts.ReadFile(args[0])
				want := args[0] + "\n"
				if got != want {
					ts.Fatalf("reading %q; got %q want %q", args[0], got, want)
				}
			},
			"testscript": func(ts *TestScript, neg bool, args []string) {
				// Run testscript in testscript. Oooh! Meta!
				fset := flag.NewFlagSet("testscript", flag.ContinueOnError)
				fUpdate := fset.Bool("update", false, "update scripts when cmp fails")
				fExplicitExec := fset.Bool("explicit-exec", false, "require explicit use of exec for commands")
				fVerbose := fset.Bool("verbose", false, "be verbose with output")
				if err := fset.Parse(args); err != nil {
					ts.Fatalf("failed to parse args for testscript: %v", err)
				}
				if fset.NArg() != 1 {
					ts.Fatalf("testscript [-verbose] [-update] [-explicit-exec] <dir>")
				}
				dir := fset.Arg(0)
				t := &fakeT{ts: ts, verbose: *fVerbose}
				func() {
					defer func() {
						if err := recover(); err != nil {
							if err != errAbort {
								panic(err)
							}
						}
					}()
					RunT(t, Params{
						Dir:                 ts.MkAbs(dir),
						UpdateScripts:       *fUpdate,
						RequireExplicitExec: *fExplicitExec,
						Cmds: map[string]func(ts *TestScript, neg bool, args []string){
							"some-param-cmd": func(ts *TestScript, neg bool, args []string) {
							},
						},
					})
				}()
				ts.stdout = strings.Replace(t.log.String(), ts.workdir, "$WORK", -1)
				if neg {
					if len(t.failMsgs) == 0 {
						ts.Fatalf("testscript unexpectedly succeeded")
					}
					return
				}
				if len(t.failMsgs) > 0 {
					ts.Fatalf("testscript unexpectedly failed with errors: %q", t.failMsgs)
				}
			},
		},
		Setup: func(env *Env) error {
			infos, err := ioutil.ReadDir(env.WorkDir)
			if err != nil {
				return fmt.Errorf("cannot read workdir: %v", err)
			}
			var setupFilenames []string
			for _, info := range infos {
				setupFilenames = append(setupFilenames, info.Name())
			}
			env.Values["setupFilenames"] = setupFilenames
			env.Values["somekey"] = 1234
			env.Values["t"] = env.T()
			env.Vars = append(env.Vars,
				"GONOSUMDB=*",
			)
			return nil
		},
	})
	if testDeferCount != 0 {
		t.Fatalf("defer mismatch; got %d want 0", testDeferCount)
	}
	// TODO check that the temp directory has been removed.
}

// TestTestwork tests that using the flag -testwork will make sure the work dir isn't removed
// after the test is done. It uses an empty testscript file that doesn't do anything.
func TestTestwork(t *testing.T) {
	out, err := exec.Command("go", "test", ".", "-testwork", "-v", "-run", "TestScripts/^nothing$").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	re := regexp.MustCompile(`\s+WORK=(\S+)`)
	match := re.FindAllStringSubmatch(string(out), -1)

	// Ensure that there is only one line with one match
	if len(match) != 1 || len(match[0]) != 2 {
		t.Fatalf("failed to extract WORK directory")
	}

	var fi os.FileInfo
	if fi, err = os.Stat(match[0][1]); err != nil {
		t.Fatalf("failed to stat expected work directory %v: %v", match[0][1], err)
	}

	if !fi.IsDir() {
		t.Fatalf("expected persisted workdir is not a directory: %v", match[0][1])
	}
}

// TestWorkdirRoot tests that a non zero value in Params.WorkdirRoot is honoured
func TestWorkdirRoot(t *testing.T) {
	td, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(td)
	params := Params{
		Dir:         filepath.Join("testdata", "nothing"),
		WorkdirRoot: td,
	}
	// Run as a sub-test so that this call blocks until the sub-tests created by
	// calling Run (which themselves call t.Parallel) complete.
	t.Run("run tests", func(t *testing.T) {
		Run(t, params)
	})
	// Verify that we have a single go-test-script-* named directory
	files, err := filepath.Glob(filepath.Join(td, "script-nothing", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("unexpected files found for kept files; got %q", files)
	}
}

// TestBadDir verifies that invoking testscript with a directory that either
// does not exist or that contains no *.txt scripts fails the test
func TestBadDir(t *testing.T) {
	ft := new(fakeT)
	func() {
		defer func() {
			if err := recover(); err != nil {
				if err != errAbort {
					panic(err)
				}
			}
		}()
		RunT(ft, Params{
			Dir: "thiswillnevermatch",
		})
	}()
	wantCount := 1
	if got := len(ft.failMsgs); got != wantCount {
		t.Fatalf("expected %v fail message; got %v", wantCount, got)
	}
	wantMsg := regexp.MustCompile(`no txtar nor txt scripts found in dir thiswillnevermatch`)
	if got := ft.failMsgs[0]; !wantMsg.MatchString(got) {
		t.Fatalf("expected msg to match `%v`; got:\n%v", wantMsg, got)
	}
}

func TestUNIX2DOS(t *testing.T) {
	for data, want := range map[string]string{
		"":         "",           // Preserve empty files.
		"\n":       "\r\n",       // Convert LF to CRLF in a file containing a single empty line.
		"\r\n":     "\r\n",       // Preserve CRLF in a single line file.
		"a":        "a\r\n",      // Append CRLF to a single line file with no line terminator.
		"a\n":      "a\r\n",      // Convert LF to CRLF in a file containing a single non-empty line.
		"a\r\n":    "a\r\n",      // Preserve CRLF in a file containing a single non-empty line.
		"a\nb\n":   "a\r\nb\r\n", // Convert LF to CRLF in multiline UNIX file.
		"a\r\nb\n": "a\r\nb\r\n", // Convert LF to CRLF in a file containing a mix of UNIX and DOS lines.
		"a\nb\r\n": "a\r\nb\r\n", // Convert LF to CRLF in a file containing a mix of UNIX and DOS lines.
	} {
		if got, err := unix2DOS([]byte(data)); err != nil || !bytes.Equal(got, []byte(want)) {
			t.Errorf("unix2DOS(%q) == %q, %v, want %q, nil", data, got, err, want)
		}
	}
}

func setSpecialVal(ts *TestScript, neg bool, args []string) {
	ts.Setenv("SPECIALVAL", "42")
}

func ensureSpecialVal(ts *TestScript, neg bool, args []string) {
	want := "42"
	if got := ts.Getenv("SPECIALVAL"); got != want {
		ts.Fatalf("expected SPECIALVAL to be %q; got %q", want, got)
	}
}

// interrupt interrupts the current background command.
// Note that this will not work under Windows.
func interrupt(ts *TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("interrupt does not support neg")
	}
	if len(args) > 0 {
		ts.Fatalf("unexpected args found")
	}
	bg := ts.BackgroundCmds()
	if got, want := len(bg), 1; got != want {
		ts.Fatalf("unexpected background cmd count; got %d want %d", got, want)
	}
	bg[0].Process.Signal(os.Interrupt)
}

func waitFile(ts *TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("waitfile does not support neg")
	}
	if len(args) != 1 {
		ts.Fatalf("usage: waitfile file")
	}
	path := ts.MkAbs(args[0])
	for i := 0; i < 100; i++ {
		_, err := os.Stat(path)
		if err == nil {
			return
		}
		if !os.IsNotExist(err) {
			ts.Fatalf("unexpected stat error: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	ts.Fatalf("timed out waiting for %q to be created", path)
}

type fakeT struct {
	ts       *TestScript
	log      bytes.Buffer
	failMsgs []string
	verbose  bool
	failed   bool
}

var errAbort = errors.New("abort test")

func (t *fakeT) Skip(args ...interface{}) {
	panic(errAbort)
}

func (t *fakeT) Fatal(args ...interface{}) {
	t.failed = true
	t.failMsgs = append(t.failMsgs, fmt.Sprint(args...))
	panic(errAbort)
}

func (t *fakeT) Parallel() {}

func (t *fakeT) Log(args ...interface{}) {
	fmt.Fprint(&t.log, args...)
}

func (t *fakeT) FailNow() {
	t.Fatal("failed")
}

func (t *fakeT) Run(name string, f func(T)) {
	f(t)
}

func (t *fakeT) Verbose() bool {
	return t.verbose
}

func (t *fakeT) Failed() bool {
	return t.failed
}
