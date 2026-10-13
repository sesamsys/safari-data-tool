package scanner

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/originparse"
)

// encodeOriginRecord builds the on-disk bytes for a single origin record using
// the layout [uint32 LE length N][1 flag byte 0x01][N UTF-8 bytes]. It is a
// local test helper for building fake origin files.
func encodeOriginRecord(text string) []byte {
	body := []byte(text)
	buf := make([]byte, 5+len(body))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(body)))
	buf[4] = 0x01
	copy(buf[5:], body)
	return buf
}

// encodeOrigin concatenates the encoding of each text into a record stream,
// pairing (scheme, host, scheme, host, ...).
func encodeOrigin(texts ...string) []byte {
	var out []byte
	for _, t := range texts {
		out = append(out, encodeOriginRecord(t)...)
	}
	return out
}

// --- Task 7.4: unit test for Scan over a fake tree -------------------------

// TestScan_FakeTree builds a fake base containing several hashed folders and
// verifies that Scan enumerates the direct child directories, sizes each one
// with du -sk (per-file 1024 rounding) semantics, resolves each folder's domain
// from a nested origin file (falling back to UnknownDomain when none exists),
// ignores non-directory entries at the base, and reports StatusOK.
//
// _Requirements: 2.1, 3.1_
func TestScan_FakeTree(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())

	// A hashed folder resolving to unifi.ui.com, with a 100-byte payload file
	// alongside its origin file. 100 bytes rounds up to 1 KB block.
	hashUnifi := "AAAA1111"
	if _, err := fake.WriteFile(hashUnifi+"/WebsiteData/https_unifi.ui.com/origin",
		encodeOrigin("https", "unifi.ui.com")); err != nil {
		t.Fatalf("seeding unifi origin: %v", err)
	}
	if _, err := fake.WriteFile(hashUnifi+"/WebsiteData/https_unifi.ui.com/payload.bin",
		make([]byte, 100)); err != nil {
		t.Fatalf("seeding unifi payload: %v", err)
	}

	// A hashed folder resolving to example.com with two files: 1000 bytes
	// (1 block) and 2000 bytes (2 blocks) -> 3 KB total, plus the origin file.
	hashExample := "BBBB2222"
	if _, err := fake.WriteFile(hashExample+"/origin",
		encodeOrigin("https", "example.com")); err != nil {
		t.Fatalf("seeding example origin: %v", err)
	}
	if _, err := fake.WriteFile(hashExample+"/a.bin", make([]byte, 1000)); err != nil {
		t.Fatalf("seeding example a.bin: %v", err)
	}
	if _, err := fake.WriteFile(hashExample+"/b.bin", make([]byte, 2000)); err != nil {
		t.Fatalf("seeding example b.bin: %v", err)
	}

	// A hashed folder with no origin file at all -> Unknown Domain. It is still
	// enumerated (an empty-of-origin folder appears with its resolved domain).
	hashNoOrigin := "CCCC3333"
	if _, err := fake.Mkdir(hashNoOrigin + "/sub"); err != nil {
		t.Fatalf("seeding no-origin folder: %v", err)
	}

	// A non-directory entry directly at the base: it must be skipped by Scan.
	if _, err := fake.WriteFile("not-a-folder.txt", []byte("ignore me")); err != nil {
		t.Fatalf("seeding stray file: %v", err)
	}

	base := fake.Root()

	folders, status, err := Scan(fake, base)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if status != model.StatusOK {
		t.Fatalf("Scan status = %v, want StatusOK", status)
	}

	// Index the results by hash for order-independent assertions.
	byHash := make(map[string]model.ScannedFolder, len(folders))
	for _, f := range folders {
		byHash[f.Hash] = f
	}

	// Exactly three directories should be enumerated; the stray file ignored.
	if len(folders) != 3 {
		t.Fatalf("Scan returned %d folders, want 3: %+v", len(folders), folders)
	}
	if _, ok := byHash["not-a-folder.txt"]; ok {
		t.Fatalf("non-directory entry was not skipped: %+v", folders)
	}

	// unifi folder: resolved domain + sized (100-byte payload -> 1 KB block;
	// the origin file adds its own block(s)).
	unifi, ok := byHash[hashUnifi]
	if !ok {
		t.Fatalf("missing folder %q in results: %+v", hashUnifi, folders)
	}
	if unifi.Domain != "unifi.ui.com" {
		t.Fatalf("unifi domain = %q, want %q", unifi.Domain, "unifi.ui.com")
	}
	if unifi.AbsPath != filepath.Join(base, hashUnifi) {
		t.Fatalf("unifi AbsPath = %q, want %q", unifi.AbsPath, filepath.Join(base, hashUnifi))
	}
	// payload.bin (100 bytes) -> 1 KB. origin file is small -> 1 KB. Total 2.
	wantUnifiKB := int64(2)
	if unifi.SizeKB != wantUnifiKB {
		t.Fatalf("unifi SizeKB = %d, want %d", unifi.SizeKB, wantUnifiKB)
	}

	// example folder: resolved domain + sized (1000->1, 2000->2, origin->1).
	example, ok := byHash[hashExample]
	if !ok {
		t.Fatalf("missing folder %q in results: %+v", hashExample, folders)
	}
	if example.Domain != "example.com" {
		t.Fatalf("example domain = %q, want %q", example.Domain, "example.com")
	}
	wantExampleKB := int64(1 + 2 + 1) // a.bin + b.bin + origin
	if example.SizeKB != wantExampleKB {
		t.Fatalf("example SizeKB = %d, want %d", example.SizeKB, wantExampleKB)
	}

	// no-origin folder: still present, resolved to UnknownDomain.
	noOrigin, ok := byHash[hashNoOrigin]
	if !ok {
		t.Fatalf("missing folder %q in results: %+v", hashNoOrigin, folders)
	}
	if noOrigin.Domain != originparse.UnknownDomain {
		t.Fatalf("no-origin domain = %q, want %q", noOrigin.Domain, originparse.UnknownDomain)
	}
	if len(noOrigin.Hosts) != 0 {
		t.Fatalf("no-origin hosts = %v, want empty", noOrigin.Hosts)
	}
}

// TestScan_EmptyFolderResolvesDomain confirms that an empty hashed folder (one
// with an origin file but no other payload) still appears in the results with
// its resolved domain and a zero-or-small size, so "no payload" never drops a
// folder from the listing.
//
// _Requirements: 2.1, 3.1_
func TestScan_EmptyFolderResolvesDomain(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())

	hash := "EMPTY0001"
	// Only the origin file exists, no other files.
	if _, err := fake.WriteFile(hash+"/origin", encodeOrigin("https", "lonely.example")); err != nil {
		t.Fatalf("seeding origin: %v", err)
	}

	folders, status, err := Scan(fake, fake.Root())
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if status != model.StatusOK {
		t.Fatalf("Scan status = %v, want StatusOK", status)
	}
	if len(folders) != 1 {
		t.Fatalf("Scan returned %d folders, want 1: %+v", len(folders), folders)
	}
	if folders[0].Domain != "lonely.example" {
		t.Fatalf("domain = %q, want %q", folders[0].Domain, "lonely.example")
	}
}

// TestScan_BaseMissing confirms that when the base path does not exist, Scan
// reports StatusNotFound with no folders and no error.
//
// _Requirements: 2.1, 3.1_
func TestScan_BaseMissing(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())

	missing := fake.Path("does/not/exist")

	folders, status, err := Scan(fake, missing)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if status != model.StatusNotFound {
		t.Fatalf("Scan status = %v, want StatusNotFound", status)
	}
	if len(folders) != 0 {
		t.Fatalf("Scan returned %d folders, want 0: %+v", len(folders), folders)
	}
}
