package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// fakePlugin is a minimal Interface implementation for the round-trip test.
type fakePlugin struct {
	mu     sync.Mutex
	closed bool
}

func (p *fakePlugin) Info() PluginInfo {
	return PluginInfo{
		RequiredEnv: map[string]bool{"WORK": true},
		ResultEnv:   map[string]bool{"GREETING": true},
		Cmds:        map[string]CmdInfo{"hello": {WritesOutput: true}},
	}
}

func (p *fakePlugin) NewTestInstance(tp TestParams) (TestInstance, error) {
	if tp.TestName == "fail" {
		return nil, fmt.Errorf("instance creation refused")
	}
	return &fakeInstance{work: tp.Env["WORK"]}, nil
}

func (p *fakePlugin) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
}

type fakeInstance struct {
	work string
}

func (i *fakeInstance) Env() map[string]string {
	return map[string]string{"GREETING": "hi from " + i.work}
}

func (i *fakeInstance) RunCmd(p CmdParams) (CmdResult, error) {
	switch p.Name {
	case "hello":
		return CmdResult{Stdout: []byte("hello " + p.Args[0] + "\n")}, nil
	case "boom":
		return CmdResult{}, fmt.Errorf("plugin transport boom")
	case "fail":
		return CmdResult{Stderr: []byte("oops\n"), Err: "exit status 1"}, nil
	}
	return CmdResult{}, fmt.Errorf("unknown command %q", p.Name)
}

func (i *fakeInstance) Close() {}

// rwc is an io.ReadWriteCloser built from a separate reader and writer, used
// to connect an in-process client and server over pipes.
type rwc struct {
	r io.ReadCloser
	w io.WriteCloser
}

func (c rwc) Read(b []byte) (int, error)  { return c.r.Read(b) }
func (c rwc) Write(b []byte) (int, error) { return c.w.Write(b) }
func (c rwc) Close() error {
	c.r.Close()
	c.w.Close()
	return nil
}

// startInProcess runs a [NewServer] loop connected to an in-process [NewClient]
// via two pipes, returning the client and a cleanup function.
func startInProcess(t *testing.T, impl Interface) (Interface, func()) {
	t.Helper()
	// client writes -> server reads
	cr, cw := io.Pipe()
	// server writes -> client reads
	sr, sw := io.Pipe()

	serveDone := make(chan struct{})
	go func() {
		NewServer(impl, rwc{r: cr, w: sw})
		close(serveDone)
	}()

	c, err := NewClient(rwc{r: sr, w: cw})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	cleanup := func() {
		c.Close()
		<-serveDone
	}
	return c, cleanup
}

func TestRoundTrip(t *testing.T) {
	impl := &fakePlugin{}
	c, cleanup := startInProcess(t, impl)
	defer cleanup()

	if cmds := c.Info().Cmds; len(cmds) != 1 || !cmds["hello"].WritesOutput {
		t.Fatalf("unexpected Info().Cmds: %v", cmds)
	}

	inst, err := c.NewTestInstance(TestParams{
		TestName: "t1",
		Env:      map[string]string{"WORK": "/tmp/work"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inst.Env()["GREETING"], "hi from /tmp/work"; got != want {
		t.Errorf("Env()[GREETING] = %q, want %q", got, want)
	}

	res, err := inst.RunCmd(CmdParams{Name: "hello", Args: []string{"world"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(res.Stdout), "hello world\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	// A command failure is reported via CmdResult.Err, not the Go error.
	res, err = inst.RunCmd(CmdParams{Name: "fail"})
	if err != nil {
		t.Fatalf("unexpected transport error for command failure: %v", err)
	}
	if res.Err != "exit status 1" {
		t.Errorf("res.Err = %q, want %q", res.Err, "exit status 1")
	}
	if got := string(res.Stderr); got != "oops\n" {
		t.Errorf("stderr = %q, want %q", got, "oops\n")
	}

	// A plugin (transport-level) failure is reported via the Go error.
	if _, err := inst.RunCmd(CmdParams{Name: "boom"}); err == nil || !strings.Contains(err.Error(), "plugin transport boom") {
		t.Errorf("RunCmd(boom) err = %v, want one containing %q", err, "plugin transport boom")
	}

	inst.Close()
}

// TestProtocolVersionTooNew checks that the client rejects a server that
// selects a protocol version newer than the client sent.
func TestProtocolVersionTooNew(t *testing.T) {
	// client writes -> server reads
	cr, cw := io.Pipe()
	// server writes -> client reads
	sr, sw := io.Pipe()

	// A hand-rolled server that answers the Info handshake with a version
	// greater than the one requested.
	go func() {
		dec := json.NewDecoder(cr)
		enc := json.NewEncoder(sw)
		var req rpcRequest
		if err := dec.Decode(&req); err != nil {
			return
		}
		enc.Encode(rpcResponse{
			ID:     req.ID,
			Result: mustMarshal(infoResult{Version: protocolVersion + 1}),
		})
	}()

	_, err := NewClient(rwc{r: sr, w: cw})
	if err == nil || !strings.Contains(err.Error(), "protocol version") {
		t.Fatalf("NewClient err = %v, want one mentioning protocol version", err)
	}
}

func TestNewInstanceError(t *testing.T) {
	c, cleanup := startInProcess(t, &fakePlugin{})
	defer cleanup()

	if _, err := c.NewTestInstance(TestParams{TestName: "fail"}); err == nil || !strings.Contains(err.Error(), "instance creation refused") {
		t.Errorf("NewTestInstance err = %v, want one containing %q", err, "instance creation refused")
	}
}

func TestConcurrentInstances(t *testing.T) {
	c, cleanup := startInProcess(t, &fakePlugin{})
	defer cleanup()

	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst, err := c.NewTestInstance(TestParams{
				TestName: fmt.Sprintf("t%d", i),
				Env:      map[string]string{"WORK": fmt.Sprintf("/w%d", i)},
			})
			if err != nil {
				t.Errorf("NewTestInstance: %v", err)
				return
			}
			defer inst.Close()
			res, err := inst.RunCmd(CmdParams{Name: "hello", Args: []string{fmt.Sprintf("%d", i)}})
			if err != nil {
				t.Errorf("RunCmd: %v", err)
				return
			}
			if got, want := string(res.Stdout), fmt.Sprintf("hello %d\n", i); got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
		}()
	}
	wg.Wait()
}
