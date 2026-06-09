package plugin

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

// Setup registers the "plugin" command into p so that test scripts can use
// plugins. Plugins are discovered, started and queried lazily the first time
// a script runs "plugin <name>".
//
// The returned cleanup function must be called when the whole test run has
// finished; it shuts down any plugin processes that were started. Because
// [testscript.Run] runs scripts as parallel subtests that execute after the
// calling test function returns, the cleanup function must be registered with
// t.Cleanup rather than deferred: a deferred call would run before the scripts
// (and the plugin processes they start) had finished, leaving processes alive.
func Setup(p *testscript.Params) (cleanup func(), err error) {
	ps := &plugins{
		started: make(map[string]*startedPlugin),
	}
	if p.Cmds == nil {
		p.Cmds = make(map[string]func(ts *testscript.TestScript, neg bool, args []string))
	}
	p.Cmds["plugin"] = ps.cmdPlugin
	return ps.close, nil
}

type plugins struct {
	// mu guards the fields below it.
	mu      sync.Mutex
	started map[string]*startedPlugin
}

type startedPlugin struct {
	client Interface
	info   PluginInfo
	proc   *procConn
}

func (ps *plugins) close() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for _, sp := range ps.started {
		sp.client.Close()
	}
	ps.started = make(map[string]*startedPlugin)
}

// get returns the started plugin with the given name, starting it on first
// use.
func (ps *plugins) get(name string) (*startedPlugin, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if sp := ps.started[name]; sp != nil {
		return sp, nil
	}
	exe := "testscript-plugin-" + name
	path, err := exec.LookPath(exe)
	if err != nil {
		return nil, fmt.Errorf("cannot find plugin executable %q: %v", exe, err)
	}
	conn, err := startProcess(path)
	if err != nil {
		return nil, fmt.Errorf("cannot start plugin %q: %v", name, err)
	}
	client, err := NewClient(conn)
	if err != nil {
		return nil, fmt.Errorf("cannot start plugin %q: %v", name, err)
	}
	sp := &startedPlugin{
		client: client,
		info:   client.Info(),
		proc:   conn,
	}
	ps.started[name] = sp
	return sp, nil
}

// startProcess starts the plugin executable at path and returns a connection
// to it: data written to the connection goes to the process's standard input,
// data read comes from its standard output, and its standard error is captured
// so it can be reported through the log of whichever test is interacting with
// the plugin. Closing the connection shuts the process down.
func startProcess(path string) (*procConn, error) {
	cmd := exec.Command(path)
	w, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := new(lockedBuffer)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &procConn{cmd: cmd, r: r, w: w, stderr: stderr}, nil
}

// procConn is the connection to a plugin subprocess, presenting its
// stdin/stdout as an [io.ReadWriteCloser].
type procConn struct {
	cmd    *exec.Cmd
	r      io.ReadCloser
	w      io.WriteCloser
	stderr *lockedBuffer
}

func (p *procConn) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *procConn) Write(b []byte) (int, error) { return p.w.Write(b) }

// Close shuts the process down: closing its standard input signals the plugin
// server loop to exit; if it does not exit promptly it is killed.
func (p *procConn) Close() error {
	p.w.Close()
	done := make(chan struct{})
	go func() {
		p.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		p.cmd.Process.Kill()
		<-done
	}
	p.r.Close()
	// The process has exited, so its standard error is fully captured. Any
	// output not already reported through a test's log (for example a crash
	// during shutdown, which belongs to no test) is written to the host's
	// standard error as a last resort so it is not lost.
	if out := p.stderr.drain(); out != "" {
		fmt.Fprintf(os.Stderr, "plugin stderr: %s", out)
	}
	return nil
}

// cmdPlugin implements the "plugin <name> [<dir>]" command. It creates a
// per-test instance of the named plugin, applies the environment variables it
// provides, and registers the commands it provides for the current test.
func (ps *plugins) cmdPlugin(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("plugin command does not support negation")
	}
	if len(args) < 1 || len(args) > 2 {
		ts.Fatalf("usage: plugin <name> [<dir>]")
	}
	name := args[0]
	sp, err := ps.get(name)
	if err != nil {
		ts.Fatalf("%v", err)
	}

	env := resolveEnv(ts, sp.info.RequiredEnv)
	dir := ""
	if len(args) > 1 {
		dir = ts.MkAbs(args[1])
	}
	inst, err := sp.client.NewTestInstance(TestParams{
		TestName:        ts.Name(),
		PluginParamsDir: dir,
		Env:             env,
	})
	logPluginStderr(ts, name, sp)
	if err != nil {
		ts.Fatalf("cannot create instance of plugin %q: %v", name, err)
	}
	ts.Defer(inst.Close)

	ps.applyEnv(ts, name, sp.info, inst.Env())
	for cmdName := range sp.info.Cmds {
		ts.SetCmd(cmdName, ps.makeCmd(name, sp, cmdName, inst))
	}
}

// makeCmd returns the testscript command implementation for a command
// provided by a plugin.
func (ps *plugins) makeCmd(name string, sp *startedPlugin, cmdName string, inst TestInstance) func(*testscript.TestScript, bool, []string) {
	cmdInfo := sp.info.Cmds[cmdName]
	return func(ts *testscript.TestScript, neg bool, args []string) {
		env := resolveEnv(ts, cmdInfo.RequiredEnv)
		res, err := inst.RunCmd(CmdParams{
			Name: cmdName,
			Args: args,
			Dir:  ts.MkAbs("."),
			Env:  env,
		})
		logPluginStderr(ts, name, sp)
		if err != nil {
			// A plugin/transport failure is fatal regardless of negation.
			ts.Fatalf("plugin %q failed to run command %q: %v", name, cmdName, err)
		}
		if cmdInfo.WritesOutput {
			ts.Stdout().Write(res.Stdout)
			ts.Stderr().Write(res.Stderr)
		}
		ps.applyEnv(ts, name, sp.info, res.Env)

		if res.Err != "" {
			ts.Logf("[%v]\n", res.Err)
			if !neg {
				ts.Fatalf("unexpected command failure")
			}
		} else if neg {
			ts.Fatalf("unexpected command success")
		}
	}
}

// applyEnv sets the environment variables provided by a plugin, rejecting any
// that the plugin did not declare in its ResultEnv.
func (ps *plugins) applyEnv(ts *testscript.TestScript, name string, info PluginInfo, env map[string]string) {
	for k, v := range env {
		if !info.ResultEnv[k] {
			ts.Fatalf("plugin %q tried to set undeclared environment variable %q", name, k)
		}
		ts.Setenv(k, v)
	}
}

// resolveEnv returns the values of the requested environment variables in the
// test environment.
func resolveEnv(ts *testscript.TestScript, required map[string]bool) map[string]string {
	env := make(map[string]string)
	for varName := range required {
		env[varName] = ts.Getenv(varName)
	}
	return env
}

// logPluginStderr reports any output the plugin process has written to its
// standard error since the last call, attributing it to the given test's log.
// Because a single plugin process is shared by all tests using it, this is
// best-effort: output is surfaced through whichever test next interacts with
// the plugin, but it keeps the output within the test framework rather than
// letting it interleave on the host's standard error.
func logPluginStderr(ts *testscript.TestScript, name string, sp *startedPlugin) {
	if out := sp.proc.stderr.drain(); out != "" {
		ts.Logf("plugin %q stderr: %s", name, out)
	}
}

// lockedBuffer is an [io.Writer], safe for concurrent use, that accumulates
// writes until drained. It captures a plugin process's standard error so it
// can be reported through a test's log.
//
// mu guards the fields below it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// drain returns any data accumulated since the last call and resets the buffer.
func (b *lockedBuffer) drain() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.buf.String()
	b.buf.Reset()
	return s
}
