package tui

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/ui"
)

// prop3RandomDomain picks a varied Domain string so the row text differs
// between folders.
func prop3RandomDomain(rng *rand.Rand) string {
	domains := []string{
		"example.com",
		"sub.example.org",
		"Unknown Domain",
		"a-really-long-domain-name-that-is-wide.example.co.uk",
		"xn--mnchen-3ya.de",
		"localhost",
	}
	return domains[rng.Intn(len(domains))]
}

// prop3RandomHash builds a random hex-ish hash string.
func prop3RandomHash(rng *rand.Rand) string {
	const hexdigits = "0123456789abcdef"
	n := 8 + rng.Intn(25) // 8..32 chars
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(hexdigits[rng.Intn(len(hexdigits))])
	}
	return b.String()
}

// prop3RandomNonEmptyFolders builds a random, non-empty slice of ScannedFolder
// values with varied Domain, Hash, and SizeKB so each FormatLine row is
// distinct across a range of sizes (KB through GB) and widths.
func prop3RandomNonEmptyFolders(rng *rand.Rand) []model.ScannedFolder {
	n := 1 + rng.Intn(15) // 1..15 folders (within the Listing_Limit)
	folders := make([]model.ScannedFolder, n)
	for i := range folders {
		folders[i] = model.ScannedFolder{
			Hash:    prop3RandomHash(rng),
			AbsPath: "/base/" + prop3RandomHash(rng),
			SizeKB:  int64(rng.Intn(5_000_000)), // 0..~5GB range
			Domain:  prop3RandomDomain(rng),
		}
	}
	return folders
}

// TestRender_Property3_RowFormatReuse validates Property 3.
//
// Feature: interactive-tui-mode, Property 3: Rendered rows reuse the existing
// line format.
//
// For any folder in a displayed Folder_List, the rendered frame contains that
// folder's ui.FormatLine(folder) text as its row. The renderer must not invent
// its own row format; it reuses the existing listing format so the TUI and the
// non-interactive listing stay consistent.
//
// **Validates: Requirements 2.5**
func TestRender_Property3_RowFormatReuse(t *testing.T) {
	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		folders := prop3RandomNonEmptyFolders(rng)
		index := rng.Intn(len(folders)) // random in-range Selection_Index

		m := Model{
			folders: folders,
			index:   index,
			mode:    modeList,
		}

		frame := string(render(m))

		for _, folder := range folders {
			line := ui.FormatLine(folder)
			if !strings.Contains(frame, line) {
				t.Errorf("frame missing FormatLine row (seed=%d, index=%d):\n want row=%q\nframe=%q",
					seed, index, line, frame)
				return false
			}
		}
		return true
	}

	// Minimum 100 iterations.
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}
