package tui

import (
	"errors"
	"io"
	"testing"
)

// stdinLikeReader models the byte stream os.Stdin produces in raw VTIME-timed
// mode. It hands back the scripted continuation bytes one at a time, then on
// the next read returns (0, io.EOF) — exactly what os.File.Read reports when a
// zero-byte timed read occurs on a descriptor built with ZeroReadIsEOF=true
// (see internal/poll.eofError). This is the read contract the bug turned into a
// fatal error.
type stdinLikeReader struct {
	script []byte
	i      int
}

func (r *stdinLikeReader) Read(p []byte) (int, error) {
	if r.i >= len(r.script) {
		return 0, io.EOF // VTIME expired with no more bytes
	}
	p[0] = r.script[r.i]
	r.i++
	return 1, nil
}

// TestTimedReadByte is the regression test for the swallowed-error crash: an
// arrow key is ESC followed by its continuation bytes, and the trailing
// VTIME-timeout read comes back from os.Stdin as io.EOF. Before the fix the
// continuation reader passed io.EOF straight through as a fatal error, so
// assembleKey returned (KeyNone, io.EOF) and the event loop quit silently.
// timedReadByte translates io.EOF into the "no more bytes" timeout signal, so
// the arrow keys decode cleanly again.
func TestTimedReadByte(t *testing.T) {
	t.Run("arrow up drains through io.EOF timeout", func(t *testing.T) {
		// ESC is the first byte (read by the blocking call in ReadKey); the
		// continuation reader sees '[' 'A' and then io.EOF.
		r := &stdinLikeReader{script: []byte{'[', 'A'}}
		key, err := assembleKey(0x1b, timedReadByte(r))
		if err != nil {
			t.Fatalf("arrow up returned error %v; expected clean decode", err)
		}
		if key != KeyUp {
			t.Fatalf("decoded %v, want KeyUp", key)
		}
	})

	t.Run("arrow down drains through io.EOF timeout", func(t *testing.T) {
		r := &stdinLikeReader{script: []byte{'[', 'B'}}
		key, err := assembleKey(0x1b, timedReadByte(r))
		if err != nil {
			t.Fatalf("arrow down returned error %v; expected clean decode", err)
		}
		if key != KeyDown {
			t.Fatalf("decoded %v, want KeyDown", key)
		}
	})

	t.Run("lone ESC resolves on immediate io.EOF", func(t *testing.T) {
		r := &stdinLikeReader{script: nil} // first continuation read is io.EOF
		key, err := assembleKey(0x1b, timedReadByte(r))
		if err != nil {
			t.Fatalf("lone ESC returned error %v; expected clean decode", err)
		}
		if key != KeyOther {
			t.Fatalf("decoded %v, want KeyOther", key)
		}
	})

	t.Run("byte available", func(t *testing.T) {
		r := &stdinLikeReader{script: []byte{'['}}
		b, ok, err := timedReadByte(r)()
		if err != nil || !ok || b != '[' {
			t.Fatalf("got (%#x, %v, %v), want ('[', true, nil)", b, ok, err)
		}
	})

	t.Run("io.EOF is a timeout, not an error", func(t *testing.T) {
		r := &stdinLikeReader{script: nil}
		b, ok, err := timedReadByte(r)()
		if err != nil || ok || b != 0 {
			t.Fatalf("got (%#x, %v, %v), want (0, false, nil)", b, ok, err)
		}
	})

	t.Run("non-EOF error is passed through", func(t *testing.T) {
		want := errors.New("boom")
		rb := timedReadByte(errReader{want})
		_, ok, err := rb()
		if err != want {
			t.Fatalf("got err %v, want %v", err, want)
		}
		if ok {
			t.Fatal("got ok=true, want false on error")
		}
	})
}

// errReader always returns the given error from Read.
type errReader struct{ err error }

func (r errReader) Read(p []byte) (int, error) { return 0, r.err }
