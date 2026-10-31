package tui

import (
	"os"
	"syscall"
	"unsafe"
)

// Terminal abstracts raw-mode terminal control and byte I/O so the event loop
// can be exercised against a FakeTerminal in tests (Req 1.4, 7.2).
type Terminal interface {
	// MakeRaw puts the terminal into unbuffered, no-echo raw mode and returns
	// a restore function that returns it to the mode captured at this call.
	MakeRaw() (restore func() error, err error)
	// ReadKey blocks for the next decoded keypress.
	ReadKey() (Key, error)
	// Write emits a rendered frame (ANSI escapes + text) to the screen.
	Write(p []byte) (int, error)
}

// Darwin termios ioctl request codes and flag masks. These values are specific
// to the macOS/Darwin ABI, which is the only supported build target (Req 1.4).
const (
	tiocGETA = 0x40487413 // TIOCGETA: read current termios
	tiocSETA = 0x80487414 // TIOCSETA: set termios

	// c_lflag (local) mode bits to clear for raw mode.
	maskECHO   = 0x8   // ECHO:   echo input characters
	maskICANON = 0x100 // ICANON: canonical (line-buffered) input
	maskISIG   = 0x80  // ISIG:   generate signals on INTR/QUIT/etc.
	maskIEXTEN = 0x400 // IEXTEN: extended input processing

	// c_iflag (input) mode bits to clear for raw mode.
	maskBRKINT = 0x2   // BRKINT
	maskICRNL  = 0x100 // ICRNL
	maskINPCK  = 0x10  // INPCK
	maskISTRIP = 0x20  // ISTRIP
	maskIXON   = 0x200 // IXON

	// c_oflag (output) mode bit to clear for raw mode.
	maskOPOST = 0x1 // OPOST: post-process output

	// Indices into the c_cc control-character array.
	ccVMIN  = 16 // VMIN:  minimum bytes for a read
	ccVTIME = 17 // VTIME: read timeout in deciseconds
)

// termios mirrors the Darwin struct termios ABI layout. On Darwin the flag
// fields and speeds are 64-bit and the control-character array has 20 entries.
type termios struct {
	Iflag  uint64
	Oflag  uint64
	Cflag  uint64
	Lflag  uint64
	Cc     [20]uint8
	Ispeed uint64
	Ospeed uint64
}

// osTerminal is the real termios-backed Terminal driver. It drives the macOS
// terminal via TIOCGETA/TIOCSETA ioctls using only the standard library
// (Req 1.4). The zero value is not usable; construct it with NewOSTerminal.
type osTerminal struct {
	fd   uintptr // terminal file descriptor (os.Stdin)
	orig termios // the termios captured at construction, restored by restore
}

// compile-time assertion that *osTerminal satisfies the Terminal interface.
var _ Terminal = (*osTerminal)(nil)

// NewOSTerminal captures the current terminal state so it can later be restored
// and returns a driver ready to enter raw mode. It fails if the current
// termios cannot be read (Req 7.2).
func NewOSTerminal() (*osTerminal, error) {
	fd := os.Stdin.Fd()
	var orig termios
	if err := ioctl(fd, tiocGETA, &orig); err != nil {
		return nil, err
	}
	return &osTerminal{fd: fd, orig: orig}, nil
}

// ioctl issues a termios ioctl against fd, passing an unsafe.Pointer to the
// Darwin-ABI termios struct. All ioctls go through syscall.Syscall so no
// third-party dependency is required (Req 1.4).
func ioctl(fd uintptr, request uintptr, t *termios) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		request,
		uintptr(unsafe.Pointer(t)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

// MakeRaw puts the terminal into unbuffered, no-echo raw mode and returns a
// restore function that re-applies the termios captured at construction
// (Req 7.2). It builds the raw variant from a fresh read of the current state
// so it reflects any changes made since construction.
func (t *osTerminal) MakeRaw() (func() error, error) {
	var raw termios
	if err := ioctl(t.fd, tiocGETA, &raw); err != nil {
		return nil, err
	}

	// Clear echo, canonical mode, signals, and extended input processing.
	raw.Lflag &^= maskECHO | maskICANON | maskISIG | maskIEXTEN
	// Clear input post-processing (break, CR/NL translation, parity, strip,
	// flow control).
	raw.Iflag &^= maskBRKINT | maskICRNL | maskINPCK | maskISTRIP | maskIXON
	// Clear output post-processing.
	raw.Oflag &^= maskOPOST
	// First-byte mode: block for at least one byte with no inter-byte timeout
	// so an idle list screen waits cleanly with no CPU spin. The ESC
	// continuation reads switch to a timed mode (see readContinuationMode) so a
	// lone ESC resolves on its own instead of blocking forever.
	raw.Cc[ccVMIN] = 1
	raw.Cc[ccVTIME] = 0

	if err := ioctl(t.fd, tiocSETA, &raw); err != nil {
		return nil, err
	}

	restore := func() error {
		orig := t.orig
		return ioctl(t.fd, tiocSETA, &orig)
	}
	return restore, nil
}

// maxEscSeqLen caps how many continuation bytes are collected after an ESC
// before decoding. A CSI arrow is ESC '[' A/B (3 bytes); longer sequences
// (function keys, mouse reports, longer CSI) are drained up to this cap so the
// byte stream stays in sync for the next keypress instead of desyncing on the
// old hard-coded 3-byte boundary. The decoder only recognizes the 3-byte arrow
// form, so anything else still decodes to KeyOther; the cap just bounds the
// drain.
const maxEscSeqLen = 8

// setReadContinuationMode switches stdin to the ESC-continuation termios mode:
// VMIN=0, VTIME=1 (0.1s inter-byte timeout). A continuation read that returns
// zero bytes then means "no more bytes arrived" (standalone ESC or end of a
// sequence) rather than blocking forever. It returns a restore func that puts
// stdin back into first-byte mode (VMIN=1, VTIME=0).
func (t *osTerminal) setReadContinuationMode() (restore func() error, err error) {
	var tc termios
	if err := ioctl(t.fd, tiocGETA, &tc); err != nil {
		return nil, err
	}
	firstByte := tc // VMIN=1, VTIME=0 as set by MakeRaw
	tc.Cc[ccVMIN] = 0
	tc.Cc[ccVTIME] = 1
	if err := ioctl(t.fd, tiocSETA, &tc); err != nil {
		return nil, err
	}
	return func() error {
		fb := firstByte
		return ioctl(t.fd, tiocSETA, &fb)
	}, nil
}

// ReadKey blocks (VMIN=1, VTIME=0) for the first byte of a keypress, then hands
// off to assembleKey. For an ESC first byte, assembleKey drains continuation
// bytes using a timed reader so a lone ESC — or any non-arrow escape sequence —
// resolves promptly instead of blocking forever (the long-run freeze fix).
func (t *osTerminal) ReadKey() (Key, error) {
	var buf [1]byte
	if _, err := os.Stdin.Read(buf[:]); err != nil {
		return KeyNone, err
	}
	if buf[0] != 0x1b {
		return decodeKey(buf[:1]), nil
	}

	// ESC first byte: switch to the timed continuation mode for the drain so a
	// standalone ESC does not park the event loop, then restore first-byte mode.
	restore, err := t.setReadContinuationMode()
	if err != nil {
		return KeyNone, err
	}
	defer restore()

	readByte := func() (byte, bool, error) {
		var next [1]byte
		n, err := os.Stdin.Read(next[:])
		if err != nil {
			return 0, false, err
		}
		if n == 0 { // VTIME expired with no byte: nothing more to read
			return 0, false, nil
		}
		return next[0], true, nil
	}
	return assembleKey(buf[0], readByte)
}

// assembleKey decodes one keypress given an already-read first byte and a
// timed continuation reader. It is the pure, TTY-free seam under test: it never
// touches os.Stdin or ioctls, so a unit test can feed it scripted bytes plus a
// timeout signal and assert the decode without a real terminal.
//
// readByte reports (b, true, nil) for a byte that arrived, (0, false, nil) when
// the inter-byte timeout expired with no byte (standalone ESC or end of a
// sequence), and (0, false, err) on a read error.
//
// For a non-ESC first byte it decodes the single byte directly. For ESC it
// collects continuation bytes up to maxEscSeqLen until the reader times out,
// then decodes the whole buffer. A lone ESC (reader times out immediately)
// returns promptly with decodeKey([]byte{0x1b}) — KeyOther today — and never
// loops forever.
func assembleKey(first byte, readByte func() (byte, bool, error)) (Key, error) {
	if first != 0x1b {
		return decodeKey([]byte{first}), nil
	}
	seq := []byte{0x1b}
	for len(seq) < maxEscSeqLen {
		b, ok, err := readByte()
		if err != nil {
			return KeyNone, err
		}
		if !ok { // timed out: lone ESC or sequence complete
			break
		}
		seq = append(seq, b)
	}
	return decodeKey(seq), nil
}

// Write passes the rendered frame straight through to stdout.
func (t *osTerminal) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}
