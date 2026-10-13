package scanner

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// --- Task 7.2: Property 11 — Access-status classification ------------------

// TestProperty11_AccessStatusClassification exercises correctness property
// "Property 11: Access-status classification" for the safari-data-tool feature.
//
// Validates: Requirements 6.1, 6.3
//
// For any (baseExists bool, childCount int >= 0):
//   - Classify returns StatusNotFound     iff !baseExists
//   - Classify returns StatusAccessBlocked iff baseExists && childCount == 0
//   - Classify returns StatusOK            iff baseExists && childCount > 0
func TestProperty11_AccessStatusClassification(t *testing.T) {
	const iterations = 200
	rng := rand.New(rand.NewSource(0x636c6173)) // seeded for reproducibility

	for i := 0; i < iterations; i++ {
		baseExists := rng.Intn(2) == 0
		// Random non-negative child count, occasionally zero to exercise the
		// blocked branch deliberately.
		var childCount int
		if rng.Intn(3) == 0 {
			childCount = 0
		} else {
			childCount = rng.Intn(10000)
		}

		got := Classify(baseExists, childCount)

		// Independently re-derive the expected status from the specification.
		var want model.AccessStatus
		switch {
		case !baseExists:
			want = model.StatusNotFound
		case childCount == 0:
			want = model.StatusAccessBlocked
		default:
			want = model.StatusOK
		}

		if got != want {
			t.Fatalf("iter %d: Classify(%v, %d) = %v, want %v",
				i, baseExists, childCount, got, want)
		}

		// Also assert the "iff" biconditionals explicitly.
		if (got == model.StatusNotFound) != !baseExists {
			t.Fatalf("iter %d: StatusNotFound should hold iff !baseExists (baseExists=%v, got=%v)",
				i, baseExists, got)
		}
		if (got == model.StatusAccessBlocked) != (baseExists && childCount == 0) {
			t.Fatalf("iter %d: StatusAccessBlocked should hold iff baseExists && childCount==0 (baseExists=%v, childCount=%d, got=%v)",
				i, baseExists, childCount, got)
		}
		if (got == model.StatusOK) != (baseExists && childCount > 0) {
			t.Fatalf("iter %d: StatusOK should hold iff baseExists && childCount>0 (baseExists=%v, childCount=%d, got=%v)",
				i, baseExists, childCount, got)
		}
	}
}

// --- Task 7.3: Property 12 — Base-path derivation --------------------------

// TestProperty12_BasePathDerivation exercises correctness property
// "Property 12: Base-path derivation" for the safari-data-tool feature.
//
// Validates: Requirement 7.1
//
// For any home directory path string,
//
//	BasePath(home) == filepath.Join(home, <container suffix>)
//
// The container suffix is re-derived here from an independent literal rather
// than the package constant, so the test genuinely pins the expected path.
func TestProperty12_BasePathDerivation(t *testing.T) {
	const iterations = 200
	rng := rand.New(rand.NewSource(0x62617365)) // seeded for reproducibility

	// Independent literal of the fixed macOS container suffix (Req 7.1). This
	// is intentionally a separate copy from scanner.containerSuffix so the
	// property cross-checks the production constant.
	const suffixLiteral = "Library/Containers/com.apple.Safari/Data/Library/WebKit/WebsiteData/Default"

	segments := []string{
		"Users", "sesam", "home", "root", "a", "bb", "ccc",
		"with space", "dot.dir", "..", ".", "trailing", "日本語", "x-y_z",
	}

	for i := 0; i < iterations; i++ {
		home := randomHome(rng, segments)

		got := BasePath(home)
		want := filepath.Join(home, suffixLiteral)

		if got != want {
			t.Fatalf("iter %d: BasePath(%q) = %q, want %q", i, home, got, want)
		}
	}
}

// randomHome builds a random home-directory path from the given segments. It
// produces a mix of absolute paths (leading separator) and relative paths, with
// a varying number of segments including the occasional empty path.
func randomHome(rng *rand.Rand, segments []string) string {
	n := rng.Intn(5) // 0..4 segments
	parts := make([]string, n)
	for i := range parts {
		parts[i] = segments[rng.Intn(len(segments))]
	}
	joined := filepath.Join(parts...)
	// Roughly half the time, make it absolute.
	if rng.Intn(2) == 0 {
		return string(filepath.Separator) + joined
	}
	return joined
}
