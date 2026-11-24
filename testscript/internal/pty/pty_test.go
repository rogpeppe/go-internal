//go:build linux || darwin

package pty

import (
	"os"
	"runtime"
	"syscall"
	"testing"
)

func TestPtyNameStackGrowth(t *testing.T) {
	var ptmx []*os.File
	for i := 0; i < 3; i++ {
		f, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
		if err != nil {
			t.Fatalf("opening /dev/ptmx: %v", err)
		}
		t.Cleanup(func() { f.Close() })
		ptmx = append(ptmx, f)
	}

	f := ptmx[2]
	want, err := ptyName(f)
	if err != nil {
		t.Fatalf("obtaining expected PTY name: %v", err)
	}
	if want == "" || want == "/dev/pts/0" {
		t.Fatalf("test PTY has unexpected name %q", want)
	}

	type result struct {
		name string
		err  error
	}
	wrong := 0
	firstDepth := 0
	var firstGot string
	for depth := 0; depth < 400; depth++ {
		ch := make(chan result, 1)
		go func() {
			name, err := ptyNameAtDepth(depth, f)
			ch <- result{name: name, err: err}
		}()
		got := <-ch
		if got.err != nil {
			t.Fatalf("depth %d: obtaining PTY name: %v", depth, got.err)
		}
		if got.name != want {
			if wrong == 0 {
				firstDepth = depth
				firstGot = got.name
			}
			wrong++
		}
	}
	if wrong != 0 {
		t.Fatalf("PTY name was wrong at %d of 400 stack depths; first at depth %d: got %q, want %q", wrong, firstDepth, firstGot, want)
	}
}

func ptyNameAtDepth(depth int, f *os.File) (string, error) {
	var pad [64]byte
	if depth > 0 {
		name, err := ptyNameAtDepth(depth-1, f)
		runtime.KeepAlive(&pad)
		return name, err
	}
	name, err := ptyName(f)
	runtime.KeepAlive(&pad)
	return name, err
}
