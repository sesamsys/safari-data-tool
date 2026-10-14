// Package ui handles human-facing formatting, the deletion confirmation
// prompt, and access-status messaging.
package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// sizeUnits is the 1024-based unit ladder. The input to FormatSize is already
// expressed in KB blocks (du -sk semantics), so KB is the base unit. B is kept
// in the ladder for graceful handling of the sub-KB / zero case.
var sizeUnits = []string{"KB", "MB", "GB", "TB", "PB"}

// FormatSize renders a size given in KB blocks (du -sk semantics) in
// human-readable units (Req 3.2).
//
// The input is already in KB, so KB is the smallest unit shown; the ladder
// climbs KB -> MB -> GB -> TB using 1024-based steps, selecting the largest
// unit under which the displayed magnitude is >= 1. The magnitude is printed as
// an integer when it is a whole number in the chosen unit, otherwise with a
// single decimal place. There is no space between the number and the unit
// (e.g. "771MB", "1.2GB", "512KB"). A zero or negative value renders as "0KB".
//
// FormatSize is monotonic non-decreasing in the underlying size (Property 8):
// larger KB values never render as a smaller human-readable size.
func FormatSize(kb int64) string {
	if kb <= 0 {
		return "0KB"
	}

	// Walk up the unit ladder while the value is large enough to warrant the
	// next unit. value is kept as a float of the current unit's magnitude.
	value := float64(kb)
	unitIdx := 0
	for value >= 1024 && unitIdx < len(sizeUnits)-1 {
		value /= 1024
		unitIdx++
	}

	unit := sizeUnits[unitIdx]

	// Integer magnitude when whole in the chosen unit, else one decimal place.
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d%s", int64(value), unit)
	}
	return fmt.Sprintf("%.1f%s", value, unit)
}

// FormatLine renders one listing row: a right-aligned 7-char human-readable
// size, a left-aligned 30-char resolved domain, and the hashed folder name in
// parentheses (Req 1.3).
func FormatLine(folder model.ScannedFolder) string {
	return fmt.Sprintf("%7s  %-30s  (%s)", FormatSize(folder.SizeKB), folder.Domain, folder.Hash)
}

// Confirm displays the target folder's resolved domain, hashed name, and
// human-readable size, then reads a single line from r and returns true only on
// an affirmative answer (Req 4.4, 4.5). Accepted affirmatives are "y" and "yes"
// (case-insensitive, surrounding whitespace trimmed); anything else, including
// EOF, returns false.
func Confirm(w io.Writer, r io.Reader, folder model.ScannedFolder) bool {
	fmt.Fprintf(w, "About to clear this site's data (permanent, no undo); a tiny origin metadata marker is kept so the folder stays identifiable:\n")
	fmt.Fprintf(w, "  Domain: %s\n", folder.Domain)
	fmt.Fprintf(w, "  Hash:   %s\n", folder.Hash)
	fmt.Fprintf(w, "  Size:   %s\n", FormatSize(folder.SizeKB))
	fmt.Fprintf(w, "Proceed? [y/N]: ")

	line, _ := bufio.NewReader(r).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// AccessMessage returns operator guidance for a non-OK AccessStatus
// (Req 6.1, 6.2, 6.3). It returns an empty string for StatusOK.
func AccessMessage(status model.AccessStatus) string {
	switch status {
	case model.StatusAccessBlocked:
		return "No Safari website-data folders were readable. This usually means " +
			"Full Disk Access is required. Grant Full Disk Access to your terminal " +
			"application in System Settings > Privacy & Security > Full Disk Access, " +
			"then try again."
	case model.StatusNotFound:
		return "The Safari website-data directory was not found."
	default:
		return ""
	}
}
