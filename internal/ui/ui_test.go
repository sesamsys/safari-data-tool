package ui

import (
	"bytes"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// propIterations is the minimum number of iterations each property-based test
// runs, satisfying the design's "minimum 100 iterations" requirement.
const propIterations = 200

// ---------------------------------------------------------------------------
// Task 8.3 - Property 3: Listing line contains required fields.
// Feature "safari-data-tool", Property 3.
// Validates: Requirements 1.3
//
// For any scanned folder (random Hash, Domain, SizeKB >= 0), the FormatLine
// output contains the formatted size (FormatSize(SizeKB)), the domain, and the
// hash as substrings.
// ---------------------------------------------------------------------------

func TestProperty3_ListingLineContainsRequiredFields(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5afa71e))

	for i := 0; i < propIterations; i++ {
		folder := model.ScannedFolder{
			Hash:   randToken(rng, 1, 40),
			Domain: randToken(rng, 0, 50),
			// Non-negative size across the full magnitude range.
			SizeKB: randNonNegativeInt64(rng),
		}

		line := FormatLine(folder)
		size := FormatSize(folder.SizeKB)

		if !strings.Contains(line, size) {
			t.Fatalf("iter %d: line %q missing formatted size %q (folder=%+v)", i, line, size, folder)
		}
		if folder.Domain != "" && !strings.Contains(line, folder.Domain) {
			t.Fatalf("iter %d: line %q missing domain %q (folder=%+v)", i, line, folder.Domain, folder)
		}
		if !strings.Contains(line, folder.Hash) {
			t.Fatalf("iter %d: line %q missing hash %q (folder=%+v)", i, line, folder.Hash, folder)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 8.4 - Property 8: Human-readable size formatting.
// Feature "safari-data-tool", Property 8.
// Validates: Requirements 3.2
//
// For any non-negative size (random int64 >= 0), FormatSize output is
// non-empty, ends with a unit suffix (one of B/KB/MB/GB/TB/PB), and is
// non-decreasing in the underlying size: for a <= b, the real magnitude
// represented by FormatSize(a) <= that of FormatSize(b).
// ---------------------------------------------------------------------------

// unitFactorsKB maps each unit suffix to its magnitude expressed in KB. The
// input to FormatSize is already in KB, so KB has factor 1. "B" is a sub-KB
// unit (1/1024 KB) kept in the ladder for the zero/sub-KB case.
var unitFactorsKB = map[string]float64{
	"B":  1.0 / 1024.0,
	"KB": 1.0,
	"MB": 1024.0,
	"GB": 1024.0 * 1024.0,
	"TB": 1024.0 * 1024.0 * 1024.0,
	"PB": 1024.0 * 1024.0 * 1024.0 * 1024.0,
}

// orderedUnits lists unit suffixes longest-first so decodeSizeToKB matches the
// most specific suffix (e.g. "KB" before "B").
var orderedUnits = []string{"KB", "MB", "GB", "TB", "PB", "B"}

// decodeSizeToKB converts a FormatSize output back to an approximate KB
// magnitude for the monotonicity comparison. It parses the leading numeric
// portion and multiplies by the matched unit's KB factor.
func decodeSizeToKB(t *testing.T, s string) float64 {
	t.Helper()
	for _, u := range orderedUnits {
		if strings.HasSuffix(s, u) {
			numPart := strings.TrimSuffix(s, u)
			val, err := strconv.ParseFloat(numPart, 64)
			if err != nil {
				t.Fatalf("decodeSizeToKB: cannot parse numeric part of %q: %v", s, err)
			}
			return val * unitFactorsKB[u]
		}
	}
	t.Fatalf("decodeSizeToKB: %q has no known unit suffix", s)
	return 0
}

func hasUnitSuffix(s string) bool {
	for _, u := range orderedUnits {
		if strings.HasSuffix(s, u) {
			return true
		}
	}
	return false
}

func TestProperty8_HumanReadableSizeFormatting(t *testing.T) {
	rng := rand.New(rand.NewSource(0x51ec08))

	for i := 0; i < propIterations; i++ {
		// Generate a sorted pair a <= b of non-negative sizes.
		a := randNonNegativeInt64(rng)
		b := randNonNegativeInt64(rng)
		if a > b {
			a, b = b, a
		}

		fa := FormatSize(a)
		fb := FormatSize(b)

		// Non-empty.
		if fa == "" || fb == "" {
			t.Fatalf("iter %d: FormatSize produced empty string (a=%d -> %q, b=%d -> %q)", i, a, fa, b, fb)
		}

		// Unit-suffixed.
		if !hasUnitSuffix(fa) {
			t.Fatalf("iter %d: FormatSize(%d)=%q has no unit suffix", i, a, fa)
		}
		if !hasUnitSuffix(fb) {
			t.Fatalf("iter %d: FormatSize(%d)=%q has no unit suffix", i, b, fb)
		}

		// Non-decreasing: decoded magnitude of the smaller input must not
		// exceed that of the larger. Allow a tiny epsilon for the rounding the
		// one-decimal display introduces.
		da := decodeSizeToKB(t, fa)
		db := decodeSizeToKB(t, fb)
		const epsilon = 1e-6
		if da > db+epsilon {
			t.Fatalf("iter %d: monotonicity violated: FormatSize(%d)=%q (%.6f KB) > FormatSize(%d)=%q (%.6f KB)",
				i, a, fa, da, b, fb, db)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 8.5 - Unit tests for Confirm and AccessMessage.
// Validates: Requirements 4.4, 4.5, 6.2
// ---------------------------------------------------------------------------

func sampleFolder() model.ScannedFolder {
	return model.ScannedFolder{
		Hash:   "ABCDEF0123456789",
		Domain: "www.example.com",
		SizeKB: 771 * 1024, // 771MB
	}
}

func TestConfirm_AffirmativeAnswers(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"lowercase y", "y\n"},
		{"word yes", "yes\n"},
		{"uppercase Y", "Y\n"},
		{"uppercase YES", "YES\n"},
		{"padded yes", "  yes  \n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w bytes.Buffer
			r := strings.NewReader(tc.input)
			if got := Confirm(&w, r, sampleFolder()); !got {
				t.Fatalf("Confirm(%q) = false, want true", tc.input)
			}
		})
	}
}

func TestConfirm_NegativeAnswers(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"lowercase n", "n\n"},
		{"empty EOF", ""},
		{"word nope", "nope\n"},
		{"word no", "no\n"},
		{"random text", "maybe\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w bytes.Buffer
			r := strings.NewReader(tc.input)
			if got := Confirm(&w, r, sampleFolder()); got {
				t.Fatalf("Confirm(%q) = true, want false", tc.input)
			}
		})
	}
}

func TestConfirm_PromptContainsFolderDetails(t *testing.T) {
	folder := sampleFolder()
	var w bytes.Buffer
	r := strings.NewReader("n\n")
	Confirm(&w, r, folder)

	prompt := w.String()
	if !strings.Contains(prompt, folder.Domain) {
		t.Errorf("prompt %q missing domain %q", prompt, folder.Domain)
	}
	if !strings.Contains(prompt, folder.Hash) {
		t.Errorf("prompt %q missing hash %q", prompt, folder.Hash)
	}
	if size := FormatSize(folder.SizeKB); !strings.Contains(prompt, size) {
		t.Errorf("prompt %q missing size %q", prompt, size)
	}
}

func TestAccessMessage_AccessBlocked(t *testing.T) {
	msg := AccessMessage(model.StatusAccessBlocked)
	if !strings.Contains(msg, "Full Disk Access") {
		t.Errorf("StatusAccessBlocked message %q missing %q", msg, "Full Disk Access")
	}
	if !strings.Contains(msg, "System Settings > Privacy & Security") {
		t.Errorf("StatusAccessBlocked message %q missing %q", msg, "System Settings > Privacy & Security")
	}
}

func TestAccessMessage_NotFound(t *testing.T) {
	msg := AccessMessage(model.StatusNotFound)
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "not found") {
		t.Errorf("StatusNotFound message %q should state the directory was not found", msg)
	}
	if !strings.Contains(lower, "website-data") && !strings.Contains(lower, "website data") {
		t.Errorf("StatusNotFound message %q should mention the website-data directory", msg)
	}
}

func TestAccessMessage_OK(t *testing.T) {
	if msg := AccessMessage(model.StatusOK); msg != "" {
		t.Errorf("AccessMessage(StatusOK) = %q, want empty string", msg)
	}
}

// ---------------------------------------------------------------------------
// Random generators shared by the property tests.
// ---------------------------------------------------------------------------

const tokenAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-"

// randToken returns a random string whose length is in [minLen, maxLen].
func randToken(rng *rand.Rand, minLen, maxLen int) string {
	n := minLen
	if maxLen > minLen {
		n += rng.Intn(maxLen - minLen + 1)
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = tokenAlphabet[rng.Intn(len(tokenAlphabet))]
	}
	return string(b)
}

// randNonNegativeInt64 returns a non-negative int64 spanning the full range of
// size magnitudes (bytes through petabyte-scale KB values), including occasional
// zero to exercise the "0KB" branch.
func randNonNegativeInt64(rng *rand.Rand) int64 {
	switch rng.Intn(10) {
	case 0:
		return 0
	case 1, 2:
		return rng.Int63n(1024) // sub-MB range in KB
	case 3, 4:
		return rng.Int63n(1024 * 1024) // up to GB range
	default:
		// Large values up to petabyte-scale KB counts.
		return rng.Int63n(1 << 50)
	}
}
