package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// Serve runs a plugin server for impl, reading JSON-RPC requests from
// [os.Stdin] and writing responses to [os.Stdout]. It is a convenience wrapper
// around [NewServer]; a plugin binary's main is typically just a call to Serve.
//
// A plugin must not write anything other than JSON-RPC frames to standard
// output. Use standard error (the default for the log package) for logging.
func Serve(impl Interface) error {
	return NewServer(impl, struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout})
}

// NewServer reads JSON-RPC requests from c and writes responses to c, invoking
// the corresponding methods of impl. It is the counterpart of [NewClient]. It
// returns when it reads EOF from c (which the host arranges by closing the
// connection when it shuts the plugin down), after calling impl.Close.
//
// Requests are dispatched concurrently, so impl's methods must be safe for
// concurrent use.
func NewServer(impl Interface, c io.ReadWriter) error {
	s := &server{impl: impl, insts: make(map[int]TestInstance)}
	dec := json.NewDecoder(c)
	enc := json.NewEncoder(c)
	var (
		wg    sync.WaitGroup
		encMu sync.Mutex
	)
	var loopErr error
	for {
		var req rpcRequest
		if err := dec.Decode(&req); err != nil {
			if err != io.EOF {
				loopErr = err
			}
			break
		}
		wg.Add(1)
		go func(req rpcRequest) {
			defer wg.Done()
			result, errStr := s.dispatch(req.Method, req.Params)
			encMu.Lock()
			enc.Encode(rpcResponse{ID: req.ID, Result: result, Error: errStr})
			encMu.Unlock()
		}(req)
	}
	wg.Wait()
	impl.Close()
	return loopErr
}

// server tracks the test instances created on behalf of a single connection.
type server struct {
	impl Interface

	// mu guards the fields below it. Per-instance work runs without the lock
	// held so that commands on different instances proceed in parallel.
	mu    sync.Mutex
	next  int
	insts map[int]TestInstance
}

// dispatch invokes the named method and returns its JSON-encoded result, or a
// non-empty error string if the method failed.
func (s *server) dispatch(method string, params json.RawMessage) (json.RawMessage, string) {
	switch method {
	case methodInfo:
		var a infoArgs
		if len(params) > 0 {
			if err := json.Unmarshal(params, &a); err != nil {
				return nil, err.Error()
			}
		}
		// Select the lower of the version we implement and the version the
		// client asked for, so the reply is never newer than the client
		// understands.
		version := min(protocolVersion, a.Version)
		return mustMarshal(infoResult{Version: version, Info: s.impl.Info()}), ""
	case methodNewInstance:
		var p TestParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err.Error()
		}
		inst, err := s.impl.NewTestInstance(p)
		if err != nil {
			return nil, err.Error()
		}
		s.mu.Lock()
		s.next++
		id := s.next
		s.insts[id] = inst
		s.mu.Unlock()
		return mustMarshal(newInstanceResult{InstID: id, Env: inst.Env()}), ""
	case methodRunCmd:
		var a runCmdArgs
		if err := json.Unmarshal(params, &a); err != nil {
			return nil, err.Error()
		}
		s.mu.Lock()
		inst := s.insts[a.InstID]
		s.mu.Unlock()
		if inst == nil {
			return nil, fmt.Sprintf("unknown plugin instance %d", a.InstID)
		}
		res, err := inst.RunCmd(a.Params)
		if err != nil {
			return nil, err.Error()
		}
		return mustMarshal(res), ""
	case methodCloseInstance:
		var a closeInstanceArgs
		if err := json.Unmarshal(params, &a); err != nil {
			return nil, err.Error()
		}
		s.mu.Lock()
		inst := s.insts[a.InstID]
		delete(s.insts, a.InstID)
		s.mu.Unlock()
		if inst != nil {
			inst.Close()
		}
		return nil, ""
	}
	return nil, fmt.Sprintf("unknown method %q", method)
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		// The values marshaled here are simple data structures that should
		// always marshal successfully.
		panic(err)
	}
	return data
}
