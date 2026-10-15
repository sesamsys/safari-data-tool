// Package cli wires command-line arguments to the tool's list and delete
// flows. Run inspects the first argument to pick a subcommand ("delete" routes
// to the delete flow; anything else, including a bare flag or no argument,
// defaults to listing) and dispatches to a testable core that takes its output
// writers, filesystem, and home directory as parameters.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sesamsys/safari-data-tool/internal/deleter"
	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
	"github.com/sesamsys/safari-data-tool/internal/selection"
	"github.com/sesamsys/safari-data-tool/internal/tui"
	"github.com/sesamsys/safari-data-tool/internal/ui"
)

// version is the executable's version string. It defaults to "dev" and is set
// at build time via
// -ldflags "-X github.com/sesamsys/safari-data-tool/internal/cli.version=...".
var version = "dev"

// defaultListingCount is the number of largest folders shown when the operator
// does not pass -n (Req 1.1).
const defaultListingCount = 15

// command classifies the invocation from its argument slice.
type command int

const (
	cmdTUI    command = iota // bare invocation / default
	cmdList                  // explicit "list" subcommand
	cmdDelete                // explicit "delete" subcommand
)

// isVersionRequest reports whether the invocation is a version request, i.e.
// the first argument is "--version" or "-v". Restricting to args[1] ensures a
// later "-v" (e.g. a subcommand flag) never triggers the version output.
func isVersionRequest(args []string) bool {
	return len(args) > 1 && (args[1] == "--version" || args[1] == "-v")
}

// classify picks the command from os.Args-style arguments. args[0] is the
// program name. A first argument of "list" or "delete" selects that
// subcommand; anything else (including no argument) selects the TUI
// (Req 1.1, 1.2, 1.3).
func classify(args []string) command {
	if len(args) > 1 {
		switch args[1] {
		case "list":
			return cmdList
		case "delete":
			return cmdDelete
		}
	}
	return cmdTUI
}

// Run is the command-line entry point. It receives the full argument slice
// (os.Args) and returns a process exit code. It wires the real operating-system
// filesystem and the operator's home directory into the testable core; tests
// drive run directly with a fake filesystem, a temp home, and captured output.
func Run(args []string) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: unable to determine home directory: %v\n", err)
		return 1
	}
	return run(args, os.Stdout, os.Stderr, os.Stdin, fsx.OS(), home)
}

// run is the testable core of the command-line entry point. It performs the
// list/delete dispatch and executes the chosen flow against the provided
// writers, standard input, filesystem, and home directory. stdin is threaded
// into both flows for a uniform core; the list flow ignores it, while the
// delete flow reads it for the confirmation prompt.
func run(args []string, stdout, stderr io.Writer, stdin io.Reader, f fsx.FileSystem, home string) int {
	if isVersionRequest(args) {
		fmt.Fprintln(stdout, "safari-data-tool "+version)
		return 0
	}
	switch classify(args) {
	case cmdList:
		return runList(args, stdout, stderr, stdin, f, home)
	case cmdDelete:
		return runDelete(args, stdout, stderr, stdin, f, home)
	default: // cmdTUI
		return runTUI(stdout, stderr, f, home)
	}
}

// runTUI launches the interactive browser (Req 1.1). It constructs the real
// termios-backed terminal and delegates to tui.Run. A terminal-construction
// failure prints to stderr and exits non-zero (Req 8.3).
func runTUI(stdout, stderr io.Writer, f fsx.FileSystem, home string) int {
	term, err := tui.NewOSTerminal()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return tui.Run(term, f, scanner.BasePath(home), stdout, stderr)
}

// runList implements the default listing flow (Req 1): parse the -n count flag,
// validate it, scan the Safari container, and print the largest folders. On a
// non-OK access status it prints only the access guidance to stderr and exits
// non-zero (Req 6.1, 6.2, 6.3).
func runList(args []string, stdout, stderr io.Writer, _ io.Reader, f fsx.FileSystem, home string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	count := fs.Int("n", defaultListingCount, "number of largest folders to display")

	// When the first argument is the literal "list" subcommand, parse the
	// arguments after it; otherwise the invocation is a bare listing and we
	// parse everything after the program name.
	var flagArgs []string
	if len(args) > 1 && args[1] == "list" {
		flagArgs = args[2:]
	} else {
		flagArgs = args[1:]
	}

	if err := fs.Parse(flagArgs); err != nil {
		// flag already wrote the parse error / usage to stderr.
		return 2
	}

	if err := selection.ValidateCount(*count); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	base := scanner.BasePath(home)
	folders, status, err := scanner.Scan(f, base)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if status != model.StatusOK {
		fmt.Fprintln(stderr, ui.AccessMessage(status))
		return 1
	}

	for _, folder := range selection.TopN(folders, *count) {
		fmt.Fprintln(stdout, ui.FormatLine(folder))
	}
	return 0
}

// runDelete implements the deletion flow (Req 4, 5): parse the selection flags,
// require at least one of --domain/--hash, scan the Safari container, match a
// single target folder, confirm (unless forced), and recursively delete it.
//
// Flag handling: --domain and --hash select the target; --force and its alias
// --yes skip the confirmation prompt (either one sets force). Providing neither
// --domain nor --hash is a usage error and exits with code 2.
//
// On a non-OK access status it prints only the access guidance to stderr and
// exits non-zero (Req 6.1, 6.2, 6.3). A no-match target prints a message and
// exits non-zero without deleting (Req 4.3). A declined confirmation exits zero
// without deleting (Req 4.5). A path-safety failure prints the error and exits
// non-zero without removing anything (Req 5.2).
func runDelete(args []string, stdout, stderr io.Writer, stdin io.Reader, f fsx.FileSystem, home string) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	domain := fs.String("domain", "", "resolved domain of the folder to delete")
	hash := fs.String("hash", "", "hashed folder name to delete")
	force := fs.Bool("force", false, "skip the confirmation prompt")
	yes := fs.Bool("yes", false, "skip the confirmation prompt (alias for --force)")

	// args[1] == "delete"; parse the arguments after the subcommand.
	if err := fs.Parse(args[2:]); err != nil {
		// flag already wrote the parse error / usage to stderr.
		return 2
	}

	// Require at least one selector (Req 4.1, 4.2).
	if *domain == "" && *hash == "" {
		fmt.Fprintln(stderr, "error: delete requires at least one of --domain or --hash")
		return 2
	}

	base := scanner.BasePath(home)
	folders, status, err := scanner.Scan(f, base)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if status != model.StatusOK {
		fmt.Fprintln(stderr, ui.AccessMessage(status))
		return 1
	}

	matched, err := deleter.Match(folders, deleter.Target{Domain: *domain, Hash: *hash})
	if err != nil {
		if errors.Is(err, deleter.ErrNoMatch) {
			fmt.Fprintln(stderr, "error: no matching folder found")
			return 1
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	// Confirm unless the operator forced the deletion (Req 4.4, 4.5, 4.7).
	if !(*force || *yes) {
		if !ui.Confirm(stdout, stdin, matched) {
			fmt.Fprintln(stdout, "Deletion cancelled.")
			return 0
		}
	}

	if err := deleter.DeletePreservingOrigin(f, base, matched.AbsPath); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Cleared %s (%s); kept its origin metadata marker.\n", matched.Domain, matched.Hash)
	return 0
}
