package tui

import (
	"testing"
	"time"
)

// scriptedReader turns a byte script into the timed continuation reader that
// assembleKey expects. Each call returns the next byte as (b, true, nil). Once
// the script is exhausted it returns (0, false, nil), which models the VTIME
// inter-byte timeout expiring with no byte — i.e. "no more bytes arrived". This
// stands in for a real TTY so the test is deterministic and never touches
// os.Stdin or an ioctl.
func scriptedReader(script []byte) func() (byte, bool, error) {
	i := 0
	return func() (byte, bool, error) {
		if i >= len(script) {
			return 0, false, nil // timeout: nothing more to read
		}
		b := script[i]
		i++
		return b, true, nil
	}
}

// runAssemble runs assembleKey under a hard timeout so a regression that
// reintroduces an indefinite block (the original freeze) fails fast instead of
// hanging the test binary.
func runAssemble(t *testing.T, first byte, reader func() (byte, bool, error)) Key {
	t.Helper()
	type result struct {
		key Key
		err error
	}
	done := make(chan result, 1)
	go func() {
		k, err := assembleKey(first, reader)
		done <- result{k, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("assembleKey returned error: %v", r.err)
		}
		return r.key
	case <-time.After(2 * time.Second):
		t.Fatal("assembleKey blocked: continuation read did not resolve on timeout (freeze regression)")
		return KeyNone
	}
}

func TestAssembleKey(t *testing.T) {
	tests := []struct {
		name  string
		first byte
		// cont is the continuation script fed after the first byte. The reader
		// returns a timeout once it is exhausted.
		cont []byte
		want Key
	}{
		{
			// The freeze scenario: a lone ESC with no continuation bytes must
			// resolve promptly via the timeout rather than blocking forever.
			name:  "lone ESC resolves on timeout without blocking",
			first: 0x1b,
			cont:  nil,
			want:  KeyOther, // bare ESC decodes to KeyOther today (ignored by the model)
		},
		{
			name:  "ESC [ A -> KeyUp",
			first: 0x1b,
			cont:  []byte{'[', 'A'},
			want:  KeyUp,
		},
		{
			name:  "ESC [ B -> KeyDown",
			first: 0x1b,
			cont:  []byte{'[', 'B'},
			want:  KeyDown,
		},
		{
			// A non-arrow CSI sequence (e.g. a function key). It must drain to
			// the timeout instead of stalling on the old 3-byte boundary. Four
			// bytes exceed the 3-byte arrow form, so it decodes to KeyOther.
			name:  "ESC [ 1 ~ drains without stalling",
			first: 0x1b,
			cont:  []byte{'[', '1', '~'},
			want:  KeyOther,
		},
		{
			// A non-ESC first byte decodes directly without consulting the
			// continuation reader at all.
			name:  "single byte q -> KeyQuit",
			first: 'q',
			cont:  nil,
			want:  KeyQuit,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := runAssemble(t, tc.first, scriptedReader(tc.cont))
			if got != tc.want {
				t.Fatalf("assembleKey(%#x, %q) = %v, want %v", tc.first, tc.cont, got, tc.want)
			}
		})
	}
}

// TestAssembleKeyDoesNotCorruptFollowingKey verifies the drain consumes the
// whole escape sequence so the next keypress starts clean. With the scripted
// reader, a sequence followed by more bytes must stop at the sequence boundary
// only via the timeout; here we assert the non-arrow sequence is fully drained
// (reader reaches its end) and still decodes deterministically.
func TestAssembleKeyDrainsEntireSequence(t *testing.T) {
	// Longer-than-arrow CSI mouse-ish report: ESC [ M followed by 3 bytes.
	cont := []byte{'[', 'M', 0x20, 0x21, 0x22}
	reader := scriptedReader(cont)
	got := runAssemble(t, 0x1b, reader)
	if got != KeyOther {
		t.Fatalf("expected non-arrow sequence to decode to KeyOther, got %v", got)
	}
	// After the drain the reader is exhausted (all continuation bytes consumed),
	// so a following read models the next keypress's first byte cleanly.
	if b, ok, _ := reader(); ok {
		t.Fatalf("continuation reader still has byte %#x; sequence was not fully drained", b)
	}
}
