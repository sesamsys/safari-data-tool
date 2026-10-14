package cli

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
)

// containerRel is the fixed container suffix, relative to the fake root, under
// which the hashed folders must live so that scanner.BasePath(home) (with
// home == fake.Root()) points into the fake tree.
const containerRel = "Library/Containers/com.apple.Safari/Data/Library/WebKit/WebsiteData/Default"

// encodeOrigin builds an origin file body: for each string a record encoded as
// [uint32 LE length N][1 flag byte 0x01][N UTF-8 bytes]. The records are laid
// out as (scheme, host) pairs; the domain resolves to the second record (host).
func encodeOrigin(records ...string) []byte {
	var buf bytes.Buffer
	for _, r := range records {
		var lenBytes [4]byte
		binary.LittleEndian.PutUint32(lenBytes[:], uint32(len(r)))
		buf.Write(lenBytes[:])
		buf.WriteByte(0x01)
		buf.WriteString(r)
	}
	return buf.Bytes()
}

// seedFolder writes an origin file plus a sibling data file for a hashed folder
// under the container suffix of the fake tree and returns the absolute path of
// the hashed folder. The data file lets tests assert that a selective delete
// clears the data while preserving the origin sidecar.
func seedFolder(t *testing.T, fake *fsx.FakeFS, hash, scheme, host string) string {
	t.Helper()
	rel := containerRel + "/" + hash + "/origin"
	if _, err := fake.WriteFile(rel, encodeOrigin(scheme, host)); err != nil {
		t.Fatalf("seed folder %s: %v", hash, err)
	}
	if _, err := fake.WriteFile(containerRel+"/"+hash+"/data.bin", make([]byte, 4096)); err != nil {
		t.Fatalf("seed folder %s data: %v", hash, err)
	}
	return fake.Path(containerRel + "/" + hash)
}

// assertCleared verifies the new default deletion contract for a hashed folder:
// the folder still exists, its origin sidecar is preserved, its sibling data
// file is gone, and a fresh scan still resolves the folder to wantDomain (not
// "Unknown Domain").
func assertCleared(t *testing.T, fake *fsx.FakeFS, folder, wantDomain string) {
	t.Helper()
	if _, err := fake.Stat(folder); err != nil {
		t.Fatalf("hashed folder should remain after clear: stat err=%v", err)
	}
	if _, err := fake.Stat(filepath.Join(folder, "origin")); err != nil {
		t.Fatalf("origin sidecar should be preserved: stat err=%v", err)
	}
	if _, err := fake.Stat(filepath.Join(folder, "data.bin")); !os.IsNotExist(err) {
		t.Fatalf("data file should be cleared: stat err=%v", err)
	}
	folders, _, err := scanner.Scan(fake, scanner.BasePath(fake.Root()))
	if err != nil {
		t.Fatalf("rescan after clear: %v", err)
	}
	var resolved string
	for _, f := range folders {
		if f.AbsPath == folder {
			resolved = f.Domain
		}
	}
	if resolved != wantDomain {
		t.Fatalf("rescan resolved folder to %q, want %q", resolved, wantDomain)
	}
}

// newPopulatedFake builds a fake FS with two hashed folders and returns it. The
// home to pass to run is fake.Root().
func newPopulatedFake(t *testing.T) *fsx.FakeFS {
	t.Helper()
	fake := fsx.NewFakeFS(t.TempDir())
	seedFolder(t, fake, "AAAA1111", "https", "example.com")
	seedFolder(t, fake, "BBBB2222", "https", "unifi.ui.com")
	return fake
}

// --- 1. Dispatch routing -------------------------------------------------

func TestRun_RoutesToListing(t *testing.T) {
	// A bare invocation now launches the TUI (Req 1.1); the listing flow is
	// reached only through the explicit "list" subcommand (Req 1.2), so these
	// routing cases all go through "list".
	cases := map[string][]string{
		"list subcmd":  {"prog", "list"},
		"list -n flag": {"prog", "list", "-n", "5"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newPopulatedFake(t)
			var stdout, stderr bytes.Buffer

			code := run(args, &stdout, &stderr, strings.NewReader(""), fake, fake.Root())

			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
			}
			out := stdout.String()
			// Listing prints a FormatLine per folder: domain + (hash).
			if !strings.Contains(out, "example.com") || !strings.Contains(out, "(AAAA1111)") {
				t.Fatalf("listing output missing seeded folders:\n%s", out)
			}
			if !strings.Contains(out, "unifi.ui.com") || !strings.Contains(out, "(BBBB2222)") {
				t.Fatalf("listing output missing seeded folders:\n%s", out)
			}
		})
	}
}

func TestRun_RoutesToDelete(t *testing.T) {
	fake := newPopulatedFake(t)
	folder := fake.Path(containerRel + "/AAAA1111")
	var stdout, stderr bytes.Buffer

	// Delete flow (not listing): force-delete by hash, empty stdin.
	code := run(
		[]string{"prog", "delete", "--hash", "AAAA1111", "--force"},
		&stdout, &stderr, strings.NewReader(""), fake, fake.Root(),
	)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	// Deletion behavior, not listing: under the new default the hashed folder
	// REMAINS (its origin sidecar is preserved) but its data is cleared, and
	// stdout reports the clear rather than a listing of both folders.
	assertCleared(t, fake, folder, "example.com")
	if !strings.Contains(stdout.String(), "Cleared") {
		t.Fatalf("expected clear confirmation on stdout, got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "BBBB2222") {
		t.Fatalf("delete flow unexpectedly listed other folders:\n%s", stdout.String())
	}
}

// --- 2. List input error -------------------------------------------------

func TestRun_ListCountNonPositive(t *testing.T) {
	for _, n := range []string{"0", "-3"} {
		t.Run("n="+n, func(t *testing.T) {
			fake := newPopulatedFake(t)
			var stdout, stderr bytes.Buffer

			code := run([]string{"prog", "list", "-n", n}, &stdout, &stderr, strings.NewReader(""), fake, fake.Root())

			if code != 2 {
				t.Fatalf("exit = %d, want 2 for -n %s", code, n)
			}
			if stdout.Len() != 0 {
				t.Fatalf("expected nothing listed on stdout, got:\n%s", stdout.String())
			}
		})
	}
}

// --- 3. Delete with neither selector -------------------------------------

func TestRun_DeleteNoSelector(t *testing.T) {
	fake := newPopulatedFake(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"prog", "delete"}, &stdout, &stderr, strings.NewReader(""), fake, fake.Root())

	if code != 2 {
		t.Fatalf("exit = %d, want 2 for delete with no selector", code)
	}
	if stderr.Len() == 0 {
		t.Fatalf("expected an input error on stderr")
	}
}

// --- 4. Delete flows -----------------------------------------------------

func TestRun_DeleteByDomainForce(t *testing.T) {
	fake := newPopulatedFake(t)
	folder := fake.Path(containerRel + "/AAAA1111")
	var stdout, stderr bytes.Buffer

	// --force skips confirmation; empty stdin proves stdin is not read (Req 4.7).
	code := run(
		[]string{"prog", "delete", "--domain", "example.com", "--force"},
		&stdout, &stderr, strings.NewReader(""), fake, fake.Root(),
	)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	// New default: the folder's data is cleared but it remains resolvable.
	assertCleared(t, fake, folder, "example.com")
}

func TestRun_DeleteByHashYesAlias(t *testing.T) {
	fake := newPopulatedFake(t)
	folder := fake.Path(containerRel + "/BBBB2222")
	var stdout, stderr bytes.Buffer

	// --yes is an alias for --force.
	code := run(
		[]string{"prog", "delete", "--hash", "BBBB2222", "--yes"},
		&stdout, &stderr, strings.NewReader(""), fake, fake.Root(),
	)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	// New default: the folder's data is cleared but it remains resolvable.
	assertCleared(t, fake, folder, "unifi.ui.com")
}

func TestRun_DeleteNoMatch(t *testing.T) {
	fake := newPopulatedFake(t)
	folderA := fake.Path(containerRel + "/AAAA1111")
	folderB := fake.Path(containerRel + "/BBBB2222")
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"prog", "delete", "--domain", "nonexistent.example", "--force"},
		&stdout, &stderr, strings.NewReader(""), fake, fake.Root(),
	)

	if code == 0 {
		t.Fatalf("exit = %d, want non-zero for no-match", code)
	}
	if stderr.Len() == 0 {
		t.Fatalf("expected a no-match message on stderr")
	}
	// Nothing deleted (Req 4.3).
	if _, err := fake.Stat(folderA); err != nil {
		t.Fatalf("folder A should still exist, stat err=%v", err)
	}
	if _, err := fake.Stat(folderB); err != nil {
		t.Fatalf("folder B should still exist, stat err=%v", err)
	}
}

func TestRun_DeleteDeclinedConfirmation(t *testing.T) {
	fake := newPopulatedFake(t)
	folder := fake.Path(containerRel + "/AAAA1111")
	var stdout, stderr bytes.Buffer

	// No --force; answer "n" to the confirmation prompt (Req 4.5).
	code := run(
		[]string{"prog", "delete", "--domain", "example.com"},
		&stdout, &stderr, strings.NewReader("n\n"), fake, fake.Root(),
	)

	if code != 0 {
		t.Fatalf("exit = %d, want 0 for declined confirmation; stderr=%q", code, stderr.String())
	}
	if _, err := fake.Stat(folder); err != nil {
		t.Fatalf("folder should NOT be deleted on decline, stat err=%v", err)
	}
}

func TestRun_DeleteConfirmed(t *testing.T) {
	fake := newPopulatedFake(t)
	folder := fake.Path(containerRel + "/AAAA1111")
	var stdout, stderr bytes.Buffer

	// No --force; answer "y" to the confirmation prompt.
	code := run(
		[]string{"prog", "delete", "--domain", "example.com"},
		&stdout, &stderr, strings.NewReader("y\n"), fake, fake.Root(),
	)

	if code != 0 {
		t.Fatalf("exit = %d, want 0 for confirmed delete; stderr=%q", code, stderr.String())
	}
	// New default: on confirm the folder's data is cleared but it remains
	// resolvable (its origin sidecar is preserved).
	assertCleared(t, fake, folder, "example.com")
}

// TestRun_DeleteNestedOriginSurvives seeds a hashed folder whose `origin`
// sidecar is deeply nested (hash/sub/dir/origin) alongside data, force-deletes
// it, and asserts the nested origin survived, the data is gone, and a fresh
// scan still resolves the folder's domain (not "Unknown Domain").
func TestRun_DeleteNestedOriginSurvives(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())
	hash := "CCCC3333"
	folder := fake.Path(containerRel + "/" + hash)
	nestedOrigin := containerRel + "/" + hash + "/sub/dir/origin"
	if _, err := fake.WriteFile(nestedOrigin, encodeOrigin("https", "nested.example")); err != nil {
		t.Fatalf("seed nested origin: %v", err)
	}
	dataRel := containerRel + "/" + hash + "/cache/data.bin"
	if _, err := fake.WriteFile(dataRel, make([]byte, 4096)); err != nil {
		t.Fatalf("seed data: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(
		[]string{"prog", "delete", "--hash", hash, "--force"},
		&stdout, &stderr, strings.NewReader(""), fake, fake.Root(),
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}

	// The deeply nested origin survived; the data is gone; the folder remains.
	if _, err := fake.Stat(folder); err != nil {
		t.Fatalf("folder should remain after clear: stat err=%v", err)
	}
	if _, err := fake.Stat(fake.Path(nestedOrigin)); err != nil {
		t.Fatalf("nested origin should be preserved: stat err=%v", err)
	}
	if _, err := fake.Stat(fake.Path(dataRel)); !os.IsNotExist(err) {
		t.Fatalf("data should be cleared: stat err=%v", err)
	}
	if _, err := fake.Stat(fake.Path(containerRel + "/" + hash + "/cache")); !os.IsNotExist(err) {
		t.Fatalf("non-origin data dir should be removed: stat err=%v", err)
	}

	// A fresh scan still resolves the folder to its domain.
	folders, _, err := scanner.Scan(fake, scanner.BasePath(fake.Root()))
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	var resolved string
	for _, f := range folders {
		if f.AbsPath == folder {
			resolved = f.Domain
		}
	}
	if resolved != "nested.example" {
		t.Fatalf("rescan resolved folder to %q, want %q", resolved, "nested.example")
	}
}

// Guard: confirm BasePath wiring assumption so the fixtures stay valid if the
// container suffix ever changes.
func TestBasePathMatchesContainerRel(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())
	if got, want := scanner.BasePath(fake.Root()), fake.Path(containerRel); got != want {
		t.Fatalf("BasePath(home)=%q, want %q", got, want)
	}
}
