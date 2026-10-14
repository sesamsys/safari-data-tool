package tui

import (
	"bytes"
	"errors"
)

// ErrScriptExhausted is returned by FakeTerminal.ReadKey once every scripted
// key has been consumed. The event loop treats a ReadKey error as a reason to
// stop, so a scripted run that forgets to end in KeyQuit still terminates
// deterministically instead of blocking forever.
var ErrScriptExhausted = errors.New("tui: fake terminal input script exhausted")

// FakeTerminal is a scripted, in-memory Terminal for tests. It mirrors the
// fsx.FakeFS pattern: a non-_test.go fake, importable by any package's tests,
// that lets the whole tui.Run loop be driven end-to-end without a real TTY.
//
// It records whether MakeRaw was called and whether the restore it returned
// ran, replays a fixed []Key from successive ReadKey calls, and captures every
// Write into a buffer so tests can assert on the rendered frames.
type FakeTerminal struct {
	// script is the sequence of keys ReadKey replays, one per call.
	script []Key
	// cursor is the index of the next scripted key ReadKey will return. It
	// advances on every ReadKey call and, once it reaches len(script), ReadKey
	// returns ErrScriptExhausted.
	cursor int

	// MakeRawCalled records that MakeRaw was invoked.
	MakeRawCalled bool
	// RestoreCalled records that the restore func returned by MakeRaw ran.
	RestoreCalled bool
	// MakeRawErr, when non-nil, is returned by MakeRaw instead of a restore
	// func, letting tests exercise the MakeRaw-failure path.
	MakeRawErr error

	// buf accumulates every byte passed to Write across the whole session.
	buf bytes.Buffer

	// writeErr, when non-nil, is returned by Write (after recording the bytes)
	// so tests can exercise the Write-failure exit path.
	writeErr error
	// readErr, when non-nil, is returned by ReadKey before the script is even
	// consulted, so tests can exercise the ReadKey-failure exit path.
	readErr error
}

// Compile-time assertion that *FakeTerminal satisfies the Terminal interface.
var _ Terminal = (*FakeTerminal)(nil)

// NewFakeTerminal returns a FakeTerminal that replays the given keys from
// successive ReadKey calls. Tests typically script a navigation/confirm/quit
// sequence:
//
//	term := tui.NewFakeTerminal(tui.KeyDown, tui.KeyDelete, tui.KeyConfirmYes, tui.KeyQuit)
func NewFakeTerminal(script ...Key) *FakeTerminal {
	return &FakeTerminal{script: script}
}

// MakeRaw records that it was called and returns a restore func that records
// its own invocation. When MakeRawErr is set it reports that failure instead,
// mirroring a termios ioctl error.
func (t *FakeTerminal) MakeRaw() (func() error, error) {
	t.MakeRawCalled = true
	if t.MakeRawErr != nil {
		return nil, t.MakeRawErr
	}
	return func() error {
		t.RestoreCalled = true
		return nil
	}, nil
}

// ReadKey returns the next scripted key. Once the script is exhausted it
// returns ErrScriptExhausted so the event loop stops deterministically. If a
// readErr has been injected it is returned first, before the script is read.
func (t *FakeTerminal) ReadKey() (Key, error) {
	if t.readErr != nil {
		return KeyNone, t.readErr
	}
	if t.cursor >= len(t.script) {
		return KeyNone, ErrScriptExhausted
	}
	k := t.script[t.cursor]
	t.cursor++
	return k, nil
}

// Write captures the frame into the internal buffer and returns the number of
// bytes recorded. When writeErr is set it is returned after the bytes are
// recorded, letting tests assert on the last frame while still exercising the
// Write-failure path.
func (t *FakeTerminal) Write(p []byte) (int, error) {
	n, _ := t.buf.Write(p)
	if t.writeErr != nil {
		return n, t.writeErr
	}
	return n, nil
}

// Output returns everything written to the terminal so far as a string, for
// frame assertions.
func (t *FakeTerminal) Output() string { return t.buf.String() }

// OutputBytes returns everything written to the terminal so far, for frame
// assertions that need raw bytes (e.g. counting ANSI escapes).
func (t *FakeTerminal) OutputBytes() []byte { return t.buf.Bytes() }

// Cursor reports how many scripted keys have been consumed by ReadKey so far.
func (t *FakeTerminal) Cursor() int { return t.cursor }

// Remaining reports how many scripted keys have not yet been consumed.
func (t *FakeTerminal) Remaining() int { return len(t.script) - t.cursor }

// FailWriteWith injects an error to be returned by Write (after the bytes are
// recorded), for exercising the Write-failure exit path. It returns the
// receiver so it can be chained onto NewFakeTerminal.
func (t *FakeTerminal) FailWriteWith(err error) *FakeTerminal {
	t.writeErr = err
	return t
}

// FailReadWith injects an error to be returned by ReadKey before the script is
// consulted, for exercising the ReadKey-failure exit path. It returns the
// receiver so it can be chained onto NewFakeTerminal.
func (t *FakeTerminal) FailReadWith(err error) *FakeTerminal {
	t.readErr = err
	return t
}
