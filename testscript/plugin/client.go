package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// NewClient returns an implementation of [Interface] that issues JSON-RPC
// requests to, and reads responses from, c. It performs an initial handshake
// to fetch the plugin's [PluginInfo]. The connection c is closed when the
// returned value's Close method is called.
//
// The returned value is safe for concurrent use; in particular the test
// instances it creates may be used concurrently.
func NewClient(c io.ReadWriteCloser) (Interface, error) {
	cl := &client{
		c:       c,
		enc:     json.NewEncoder(c),
		pending: make(map[uint64]chan rpcResponse),
	}
	go cl.readLoop()
	var res infoResult
	if err := cl.call(methodInfo, infoArgs{Version: protocolVersion}, &res); err != nil {
		c.Close()
		return nil, fmt.Errorf("plugin handshake failed: %v", err)
	}
	if res.Version > protocolVersion {
		c.Close()
		return nil, fmt.Errorf("plugin selected protocol version %d, greater than supported version %d", res.Version, protocolVersion)
	}
	cl.info = res.Info
	return cl, nil
}

type client struct {
	c    io.ReadWriteCloser
	info PluginInfo

	// mu guards the fields below it.
	mu      sync.Mutex
	enc     *json.Encoder
	seq     uint64
	pending map[uint64]chan rpcResponse
	err     error // set once the read loop terminates
}

// readLoop reads responses from the connection and delivers each to the
// goroutine waiting on its request id. When the connection fails it records
// the error and wakes all waiting callers.
func (cl *client) readLoop() {
	dec := json.NewDecoder(cl.c)
	for {
		var resp rpcResponse
		if err := dec.Decode(&resp); err != nil {
			cl.mu.Lock()
			cl.err = err
			pending := cl.pending
			cl.pending = nil
			cl.mu.Unlock()
			for _, ch := range pending {
				close(ch)
			}
			return
		}
		cl.mu.Lock()
		ch := cl.pending[resp.ID]
		delete(cl.pending, resp.ID)
		cl.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

// call sends a request and waits for its response, unmarshaling the result
// into result (if non-nil). A non-empty response error is returned as an error.
// TODO we could pass a context in here to enable timing out.
func (cl *client) call(method string, arg, result any) error {
	var params json.RawMessage
	if arg != nil {
		data, err := json.Marshal(arg)
		if err != nil {
			return err
		}
		params = data
	}
	ch := make(chan rpcResponse, 1)
	cl.mu.Lock()
	if cl.err != nil {
		err := cl.err
		cl.mu.Unlock()
		return err
	}
	cl.seq++
	id := cl.seq
	cl.pending[id] = ch
	err := cl.enc.Encode(rpcRequest{ID: id, Method: method, Params: params})
	if err != nil {
		delete(cl.pending, id)
		cl.mu.Unlock()
		return err
	}
	cl.mu.Unlock()

	resp, ok := <-ch
	if !ok {
		// The read loop closed the channel: the connection failed.
		cl.mu.Lock()
		err := cl.err
		cl.mu.Unlock()
		if err == nil || err == io.EOF {
			err = errors.New("plugin connection closed")
		}
		return err
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	if result != nil && len(resp.Result) > 0 {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

func (cl *client) Info() PluginInfo { return cl.info }

func (cl *client) NewTestInstance(p TestParams) (TestInstance, error) {
	var res newInstanceResult
	if err := cl.call(methodNewInstance, p, &res); err != nil {
		return nil, err
	}
	return &clientInstance{cl: cl, instID: res.InstID, env: res.Env}, nil
}

// Close closes the underlying connection, which signals the plugin server to
// shut down.
func (cl *client) Close() {
	cl.c.Close()
}

type clientInstance struct {
	cl     *client
	instID int
	env    map[string]string
}

func (i *clientInstance) Env() map[string]string { return i.env }

func (i *clientInstance) RunCmd(p CmdParams) (CmdResult, error) {
	var res CmdResult
	if err := i.cl.call(methodRunCmd, runCmdArgs{InstID: i.instID, Params: p}, &res); err != nil {
		return CmdResult{}, err
	}
	return res, nil
}

func (i *clientInstance) Close() {
	i.cl.call(methodCloseInstance, closeInstanceArgs{InstID: i.instID}, nil)
}
