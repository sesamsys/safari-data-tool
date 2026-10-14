package cli

import (
	"bytes"
	"encoding/binary"
	"os"
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

// seedFolder writes an origin file for a hashed folder under the container
// suffix of the fake tree and returns the absolute path of the hashed folder.
func seedFolder(t *testing.T, fake *fsx.FakeFS, hash, scheme, host string) string {
	t.Helper()
	rel := containerRel + "/" + hash + "/origin"
	if _, err := fake.WriteFile(rel, encodeOrigin(scheme, host)); err != nil {
		t.Fatalf("seed folder %s: %v", hash, err)
	}
	return fake.Path(containerRel + "/" + hash)
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
	// Deletion behavior, not listing: the folder is gone and stdout reports
	// the delete rather than a listing of both folders.
	if _, err := fake.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("folder still exists after delete routing: stat err=%v", err)
	}
	if !strings.Contains(stdout.String(), "Deleted") {
		t.Fatalf("expected delete confirmation on stdout, got:\n%s", stdout.String())
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
	if _, err := fake.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("folder should be deleted, stat err=%v", err)
	}
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
	if _, err := fake.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("folder should be deleted, stat err=%v", err)
	}
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
	if _, err := fake.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("folder should be deleted on confirm, stat err=%v", err)
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
