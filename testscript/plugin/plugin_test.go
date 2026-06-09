package plugin_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/rogpeppe/go-internal/testscript/plugin"
)

func TestMain(m *testing.M) {
	// Register the test plugins as commands so that testscript.Main makes
	// them available on $PATH under their "testscript-plugin-<name>" names,
	// which is how the plugin host discovers them.
	testscript.Main(m, map[string]func(){
		"testscript-plugin-demo": func() {
			if err := plugin.Serve(demoPlugin{}); err != nil {
				log.Fatal(err)
			}
		},
		"testscript-plugin-kv": func() {
			if err := plugin.Serve(kvPlugin{}); err != nil {
				log.Fatal(err)
			}
		},
	})
}

func TestPlugin(t *testing.T) {
	p := testscript.Params{
		Dir: "testdata",
	}
	cleanup, err := plugin.Setup(&p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	testscript.Run(t, p)
}

// demoPlugin exercises most of the plugin contract: it reads its parameter
// directory and a variable from the test environment when an instance is
// created, exposes result variables, and provides commands that produce
// standard output and standard error, run in the script's working directory,
// set environment variables, and fail.
type demoPlugin struct{}

func (demoPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		RequiredEnv: map[string]bool{"GREETING": true},
		ResultEnv:   map[string]bool{"DEMO_MESSAGE": true, "DEMO_TOKEN": true},
		Cmds: map[string]plugin.CmdInfo{
			"echo":     {WritesOutput: true},
			"warn":     {WritesOutput: true},
			"cwd":      {WritesOutput: true},
			"settoken": {},
			"fail":     {WritesOutput: true},
		},
	}
}

func (demoPlugin) NewTestInstance(p plugin.TestParams) (plugin.TestInstance, error) {
	name := "anonymous"
	if p.PluginParamsDir != "" {
		data, err := os.ReadFile(filepath.Join(p.PluginParamsDir, "name.txt"))
		if err != nil {
			return nil, fmt.Errorf("cannot read plugin parameter directory: %v", err)
		}
		name = strings.TrimSpace(string(data))
	}
	greeting := p.Env["GREETING"]
	if greeting == "" {
		greeting = "hello"
	}
	return &demoInstance{message: greeting + ", " + name}, nil
}

func (demoPlugin) Close() {}

type demoInstance struct {
	message string
}

func (inst *demoInstance) Env() map[string]string {
	return map[string]string{"DEMO_MESSAGE": inst.message}
}

func (inst *demoInstance) RunCmd(p plugin.CmdParams) (plugin.CmdResult, error) {
	switch p.Name {
	case "echo":
		return plugin.CmdResult{Stdout: []byte(strings.Join(p.Args, " ") + "\n")}, nil
	case "warn":
		return plugin.CmdResult{Stderr: []byte(strings.Join(p.Args, " ") + "\n")}, nil
	case "cwd":
		return plugin.CmdResult{Stdout: []byte(p.Dir + "\n")}, nil
	case "settoken":
		if len(p.Args) != 1 {
			return plugin.CmdResult{}, fmt.Errorf("usage: settoken <value>")
		}
		return plugin.CmdResult{Env: map[string]string{"DEMO_TOKEN": p.Args[0]}}, nil
	case "fail":
		return plugin.CmdResult{Stderr: []byte("deliberate failure\n"), Err: "exit status 1"}, nil
	}
	return plugin.CmdResult{}, fmt.Errorf("unrecognized command %q", p.Name)
}

func (inst *demoInstance) Close() {}

// kvPlugin is a minimal plugin invoked without a parameter directory. Each test
// instance holds its own in-memory key-value store, demonstrating that an
// instance persists state across the commands of a single test (the classic
// "scratch server" use case).
type kvPlugin struct{}

func (kvPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Cmds: map[string]plugin.CmdInfo{
			"set": {},
			"get": {WritesOutput: true},
		},
	}
}

func (kvPlugin) NewTestInstance(p plugin.TestParams) (plugin.TestInstance, error) {
	return &kvInstance{store: make(map[string]string)}, nil
}

func (kvPlugin) Close() {}

type kvInstance struct {
	// mu guards the fields below it.
	mu    sync.Mutex
	store map[string]string
}

func (inst *kvInstance) Env() map[string]string { return nil }

func (inst *kvInstance) RunCmd(p plugin.CmdParams) (plugin.CmdResult, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	switch p.Name {
	case "set":
		if len(p.Args) != 2 {
			return plugin.CmdResult{}, fmt.Errorf("usage: set <key> <value>")
		}
		inst.store[p.Args[0]] = p.Args[1]
		return plugin.CmdResult{}, nil
	case "get":
		if len(p.Args) != 1 {
			return plugin.CmdResult{}, fmt.Errorf("usage: get <key>")
		}
		return plugin.CmdResult{Stdout: []byte(inst.store[p.Args[0]] + "\n")}, nil
	}
	return plugin.CmdResult{}, fmt.Errorf("unrecognized command %q", p.Name)
}

func (inst *kvInstance) Close() {}
