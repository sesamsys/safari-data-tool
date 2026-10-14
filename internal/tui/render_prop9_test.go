package tui

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/ui"
)

// prop9RandomDomain picks a varied Domain string, including the wide/unicode
// and "Unknown Domain" cases, so the confirmation detail set is exercised
// across realistic resolved-domain values.
func prop9RandomDomain(rng *rand.Rand) string {
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

// prop9RandomHash builds a random hex-ish hash string of varied length so the
// Hash shown in the confirmation differs run to run.
func prop9RandomHash(rng *rand.Rand) string {
	const hexdigits = "0123456789abcdef"
	n := 8 + rng.Intn(25) // 8..32 chars
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(hexdigits[rng.Intn(len(hexdigits))])
	}
	return b.String()
}

// prop9RandomSizeKB returns a size spanning the KB..GB range so the confirmed
// size exercises every rung of ui.FormatSize's unit ladder, including the
// zero case.
func prop9RandomSizeKB(rng *rand.Rand) int64 {
	switch rng.Intn(4) {
	case 0:
		return 0 // "0KB"
	case 1:
		return int64(rng.Intn(1024)) // sub-MB, KB unit
	case 2:
		return int64(rng.Intn(5_000_000)) // KB..GB range
	default:
		return int64(rng.Intn(2_000_000_000)) // up to ~TB
	}
}

// prop9RandomFolder builds a single ScannedFolder with varied Domain, Hash, and
// SizeKB — the three details the confirmation prompt must show.
func prop9RandomFolder(rng *rand.Rand) model.ScannedFolder {
	hash := prop9RandomHash(rng)
	return model.ScannedFolder{
		Hash:    hash,
		AbsPath: "/base/" + hash,
		SizeKB:  prop9RandomSizeKB(rng),
		Domain:  prop9RandomDomain(rng),
	}
}

// Feature: interactive-tui-mode, Property 9: Confirmation shows Domain, Hash, and Size
//
// For any folder, the confirmation frame rendered for it contains that folder's
// Domain, its Hash, and its size as rendered by ui.FormatSize(SizeKB). The TUI
// enters modeConfirm with the random folder as the selected item (index into a
// list whose selected slot holds that folder); render in modeConfirm appends
// renderConfirm(folders[index]), which carries the Domain/Hash/Size detail set
// matching ui.Confirm.
//
// **Validates: Requirements 5.2**
func TestRender_Property9_ConfirmationDetailSet(t *testing.T) {
	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// The folder under confirmation.
		target := prop9RandomFolder(rng)

		// Place the target at a random index within a non-empty list so the
		// confirmation targets a varied Selection_Index, with other folders
		// surrounding it.
		n := 1 + rng.Intn(15) // 1..15 folders (within the Listing_Limit)
		index := rng.Intn(n)
		folders := make([]model.ScannedFolder, n)
		for i := range folders {
			folders[i] = prop9RandomFolder(rng)
		}
		folders[index] = target

		m := Model{
			folders: folders,
			index:   index,
			mode:    modeConfirm,
		}

		frame := string(render(m))

		if !strings.Contains(frame, target.Domain) {
			t.Errorf("confirmation frame missing Domain (seed=%d, index=%d): want %q\nframe=%q",
				seed, index, target.Domain, frame)
			return false
		}
		if !strings.Contains(frame, target.Hash) {
			t.Errorf("confirmation frame missing Hash (seed=%d, index=%d): want %q\nframe=%q",
				seed, index, target.Hash, frame)
			return false
		}
		wantSize := ui.FormatSize(target.SizeKB)
		if !strings.Contains(frame, wantSize) {
			t.Errorf("confirmation frame missing Size (seed=%d, index=%d): want %q\nframe=%q",
				seed, index, wantSize, frame)
			return false
		}
		return true
	}

	// Minimum 100 iterations.
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}
