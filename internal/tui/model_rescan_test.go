package tui

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
)

// containerSuffix mirrors scanner's fixed macOS container path so the test can
// seed hashed child folders where scanner.Scan expects to find them.
const containerSuffix = "Library/Containers/com.apple.Safari/Data/Library/WebKit/WebsiteData/Default"

// encodeOrigin builds an origin-file body as (scheme, host) pair records in the
// [uint32 LE length N][1 flag byte 0x01][N UTF-8 bytes] scheme so a seeded
// folder resolves to the given host.
func encodeOrigin(scheme, host string) []byte {
	var buf bytes.Buffer
	for _, r := range []string{scheme, host} {
		var lenBytes [4]byte
		binary.LittleEndian.PutUint32(lenBytes[:], uint32(len(r)))
		buf.Write(lenBytes[:])
		buf.WriteByte(0x01)
		buf.WriteString(r)
	}
	return buf.Bytes()
}

// TestApplyDelete_RescanAfterSuccessfulDeletion exercises the re-scan-and-rebuild
// path of applyDelete end-to-end against a Safari-shaped fake tree, under the
// new default where deletion preserves the `origin` sidecar.
//
// Each hashed folder is seeded with BOTH a data file AND an `origin` file. After
// deleting the selected folder its data is cleared but the sidecar remains, so
// the folder STILL appears in the rebuilt list (same folder count) at a smaller
// size and still resolves to its seeded domain (Req 6.1, 6.2, 6.3).
//
// _Requirements: 6.1, 6.2, 6.3_
func TestApplyDelete_RescanAfterSuccessfulDeletion(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())

	// Seed hashed child folders with data files of distinct sizes (so the
	// descending ordering is deterministic) plus an origin sidecar each.
	//
	//   aaa -> ~3 KB data  (largest)
	//   bbb -> ~2 KB data
	//   ccc -> ~1 KB data  (smallest)
	seed := []struct {
		hash  string
		host  string
		bytes int
	}{
		{"aaa", "aaa.example", 3000},
		{"bbb", "bbb.example", 2000},
		{"ccc", "ccc.example", 1000},
	}
	for _, s := range seed {
		dataRel := filepath.Join(containerSuffix, s.hash, "data.bin")
		if _, err := fake.WriteFile(dataRel, make([]byte, s.bytes)); err != nil {
			t.Fatalf("seeding %s data: %v", s.hash, err)
		}
		originRel := filepath.Join(containerSuffix, s.hash, "origin")
		if _, err := fake.WriteFile(originRel, encodeOrigin("https", s.host)); err != nil {
			t.Fatalf("seeding %s origin: %v", s.hash, err)
		}
	}

	base := scanner.BasePath(fake.Root())

	// Build the initial Model from a real scan of the seeded tree.
	folders, status, err := scanner.Scan(fake, base)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	m := newModel(folders, status)

	if len(m.folders) != len(seed) {
		t.Fatalf("expected %d folders after initial scan, got %d", len(seed), len(m.folders))
	}

	// Target the second-largest folder (bbb) to make the index non-trivial.
	m.index = 1
	target := m.folders[m.index]

	rebuilt := applyDelete(m, fake, base)

	// The folder still appears after a selective delete (its origin sidecar was
	// preserved), so the count is unchanged (Req 6.1).
	if got, want := len(rebuilt.folders), len(seed); got != want {
		t.Fatalf("expected %d folders after selective deletion, got %d", want, got)
	}

	// Locate the deleted folder in the rebuilt list: it must still be present,
	// its data gone (size shrank), and its domain preserved.
	var found bool
	for _, f := range rebuilt.folders {
		if f.AbsPath != target.AbsPath {
			continue
		}
		found = true
		if f.SizeKB >= target.SizeKB {
			t.Fatalf("deleted folder size did not shrink: before=%d after=%d", target.SizeKB, f.SizeKB)
		}
		if f.Domain != target.Domain {
			t.Fatalf("deleted folder domain changed: before=%q after=%q", target.Domain, f.Domain)
		}
	}
	if !found {
		t.Fatalf("deleted folder %q should still be present after selective delete", target.AbsPath)
	}

	// The rebuilt list honours the Listing_Limit and descending-size ordering
	// (Req 6.2).
	if len(rebuilt.folders) > listingLimit {
		t.Fatalf("rebuilt list length %d exceeds listingLimit %d", len(rebuilt.folders), listingLimit)
	}
	for i := 1; i < len(rebuilt.folders); i++ {
		if rebuilt.folders[i-1].SizeKB < rebuilt.folders[i].SizeKB {
			t.Fatalf("rebuilt list not ordered by descending SizeKB at index %d: %d < %d",
				i, rebuilt.folders[i-1].SizeKB, rebuilt.folders[i].SizeKB)
		}
	}

	// The Selection_Index is in range for the rebuilt, non-empty list (Req 6.3).
	if rebuilt.index < 0 || rebuilt.index >= len(rebuilt.folders) {
		t.Fatalf("selection index %d out of range for rebuilt list of length %d",
			rebuilt.index, len(rebuilt.folders))
	}
}
