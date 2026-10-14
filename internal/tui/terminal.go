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
	// Block for at least one byte with no inter-byte timeout.
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

// ReadKey reads one byte from stdin and, when it is ESC (0x1B), reads up to two
// more bytes to complete an arrow escape sequence, then delegates to decodeKey.
func (t *osTerminal) ReadKey() (Key, error) {
	var buf [1]byte
	if _, err := os.Stdin.Read(buf[:]); err != nil {
		return KeyNone, err
	}
	if buf[0] != 0x1b {
		return decodeKey(buf[:1]), nil
	}

	// ESC: read up to two more bytes to form the three-byte CSI sequence.
	seq := []byte{0x1b}
	var next [1]byte
	for len(seq) < 3 {
		n, err := os.Stdin.Read(next[:])
		if err != nil {
			return KeyNone, err
		}
		if n == 0 {
			break
		}
		seq = append(seq, next[0])
	}
	return decodeKey(seq), nil
}

// Write passes the rendered frame straight through to stdout.
func (t *osTerminal) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}
