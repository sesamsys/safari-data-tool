package tui

import (
	"fmt"
	"io"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
	"github.com/sesamsys/safari-data-tool/internal/ui"
)

// Run establishes raw mode, performs the initial scan, and drives the
// read-render loop until the operator quits. It restores the terminal on every
// exit path via defer (Req 7.2) and returns a non-zero code for a non-OK access
// status (Req 8.3).
func Run(term Terminal, f fsx.FileSystem, base string, stdout, stderr io.Writer) int {
	folders, status, err := scanner.Scan(f, base) // scan before rendering (Req 2.1)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if status != model.StatusOK {
		fmt.Fprintln(stderr, ui.AccessMessage(status)) // Req 8.1, 8.2
		return 1                                        // Req 8.3
	}

	restore, err := term.MakeRaw()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer restore() // Req 7.2 — runs on quit, error, or panic

	m := newModel(folders, status)
	for {
		if _, err := term.Write(render(m)); err != nil {
			return 1
		}
		key, err := term.ReadKey()
		if err != nil {
			return 1
		}
		var act action
		m, act = m.handleKey(key)
		switch act {
		case actQuit:
			return 0 // Req 7.1
		case actDelete:
			m = applyDelete(m, f, base)
		}
	}
}
