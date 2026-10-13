package originparse

import (
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
)

// encodeRecord builds the on-disk bytes for a single origin record using the
// layout [uint32 LE length N][1 flag byte][N UTF-8 bytes]. It is a local test
// helper for constructing record streams.
func encodeRecord(flag byte, text string) []byte {
	body := []byte(text)
	buf := make([]byte, recordHeaderSize+len(body))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(body)))
	buf[4] = flag
	copy(buf[recordHeaderSize:], body)
	return buf
}

// encodeRecords concatenates the encoding of each text as a record with the
// standard flag byte 0x01.
func encodeRecords(texts ...string) []byte {
	var out []byte
	for _, t := range texts {
		out = append(out, encodeRecord(0x01, t)...)
	}
	return out
}

// --- Task 3.2: table-driven unit tests ------------------------------------

func TestResolveDomain_TableDriven(t *testing.T) {
	// The real unifi.ui.com fixture, written out by hand as raw bytes to match
	// what Safari stores on disk:
	//   [05 00 00 00][01]"https"[0C 00 00 00][01]"unifi.ui.com"
	// The host "unifi.ui.com" is 12 bytes, so its little-endian length prefix
	// is 0x0C.
	unifiFixture := []byte{
		0x05, 0x00, 0x00, 0x00, 0x01, 'h', 't', 't', 'p', 's',
		0x0C, 0x00, 0x00, 0x00, 0x01,
		'u', 'n', 'i', 'f', 'i', '.', 'u', 'i', '.', 'c', 'o', 'm',
	}

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "real unifi.ui.com hex fixture",
			data: unifiFixture,
			want: "unifi.ui.com",
		},
		{
			name: "two clean records resolve to the host",
			data: encodeRecords("https", "www.gstatic.com"),
			want: "www.gstatic.com",
		},
		{
			name: "empty input yields Unknown Domain",
			data: nil,
			want: UnknownDomain,
		},
		{
			name: "single record (fewer than two) yields Unknown Domain",
			data: encodeRecords("https"),
			want: UnknownDomain,
		},
		{
			name: "truncated length-prefixed record yields Unknown Domain",
			// Valid first record, then a header declaring 11 bytes but the text
			// is cut short -> only one record decodes before ErrMalformed.
			data: append(encodeRecords("https"),
				0x0B, 0x00, 0x00, 0x00, 0x01, 'u', 'n', 'i'),
			want: UnknownDomain,
		},
		{
			name: "partial header mid-stream yields Unknown Domain",
			data: append(encodeRecords("https"), 0x02, 0x00),
			want: UnknownDomain,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveDomain(tc.data)
			if got != tc.want {
				t.Fatalf("ResolveDomain() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFindOriginFileAndResolve_FakeTree(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())

	// A hashed folder with the origin file nested a couple levels deep.
	hash := "ABCDEF0123456789"
	originRel := hash + "/WebsiteData/https_unifi.ui.com/origin"
	if _, err := fake.WriteFile(originRel, encodeRecords("https", "unifi.ui.com")); err != nil {
		t.Fatalf("seeding origin file: %v", err)
	}

	folder := fake.Path(hash)

	gotPath := FindOriginFile(fake, folder)
	wantPath := fake.Path(originRel)
	if gotPath != wantPath {
		t.Fatalf("FindOriginFile() = %q, want %q", gotPath, wantPath)
	}

	domain, hosts := Resolve(fake, folder)
	if domain != "unifi.ui.com" {
		t.Fatalf("Resolve() domain = %q, want %q", domain, "unifi.ui.com")
	}
	if len(hosts) != 1 || hosts[0] != "unifi.ui.com" {
		t.Fatalf("Resolve() hosts = %v, want [unifi.ui.com]", hosts)
	}
}

func TestResolve_NoOriginFile(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())
	hash := "DEADBEEF"
	if _, err := fake.Mkdir(hash + "/WebsiteData"); err != nil {
		t.Fatalf("seeding folder: %v", err)
	}

	domain, hosts := Resolve(fake, fake.Path(hash))
	if domain != UnknownDomain {
		t.Fatalf("Resolve() domain = %q, want %q", domain, UnknownDomain)
	}
	if hosts != nil {
		t.Fatalf("Resolve() hosts = %v, want nil", hosts)
	}
}

// --- Task 3.3: Property 4 — Origin record round-trip -----------------------

// TestProperty4_OriginRecordRoundTrip exercises correctness property
// "Property 4: Origin record round-trip" for the safari-data-tool feature.
//
// Validates: Requirements 2.2
//
// For any sequence of record texts, encoding each with the layout
// [uint32 LE length][flag byte 0x01][N UTF-8 bytes] and then calling
// ParseRecords yields back the original sequence of texts.
func TestProperty4_OriginRecordRoundTrip(t *testing.T) {
	const iterations = 200
	rng := rand.New(rand.NewSource(0x4b69726f)) // seeded for reproducibility

	for i := 0; i < iterations; i++ {
		// Random sequence of 0..8 record texts, each 0..20 random UTF-8 runes.
		count := rng.Intn(9)
		texts := make([]string, count)
		for j := range texts {
			texts[j] = randomText(rng, rng.Intn(21))
		}

		data := encodeRecords(texts...)
		records, err := ParseRecords(data)
		if err != nil {
			t.Fatalf("iter %d: ParseRecords returned error on well-formed input: %v (texts=%q)", i, err, texts)
		}
		if len(records) != len(texts) {
			t.Fatalf("iter %d: got %d records, want %d (texts=%q)", i, len(records), len(texts), texts)
		}
		for j := range texts {
			if records[j].Text != texts[j] {
				t.Fatalf("iter %d: record %d = %q, want %q", i, j, records[j].Text, texts[j])
			}
		}
	}
}

// --- Task 3.4: Property 5 — First-pair host resolution ---------------------

// TestProperty5_FirstPairHostResolution exercises correctness property
// "Property 5: First-pair host resolution" for the safari-data-tool feature.
//
// Validates: Requirements 2.1, 2.3
//
// For any origin byte stream whose first two records decode to a scheme
// followed by a host (with optional extra records appended), ResolveDomain
// returns that host.
func TestProperty5_FirstPairHostResolution(t *testing.T) {
	const iterations = 200
	rng := rand.New(rand.NewSource(0x686f7374)) // seeded for reproducibility

	for i := 0; i < iterations; i++ {
		scheme := randomText(rng, 1+rng.Intn(8))
		host := randomText(rng, 1+rng.Intn(20))

		texts := []string{scheme, host}
		// Optionally append extra scheme/host pairs; they must not change the
		// primary resolution (the host of the first pair).
		extra := rng.Intn(5)
		for e := 0; e < extra; e++ {
			texts = append(texts, randomText(rng, rng.Intn(10)))
		}

		data := encodeRecords(texts...)
		got := ResolveDomain(data)
		if got != host {
			t.Fatalf("iter %d: ResolveDomain = %q, want host %q (texts=%q)", i, got, host, texts)
		}
	}
}

// --- Task 3.5: Property 6 — Malformed origin robustness --------------------

// TestProperty6_MalformedOriginRobustness exercises correctness property
// "Property 6: Malformed origin robustness" for the safari-data-tool feature.
//
// Validates: Requirements 2.4
//
// For any truncated/malformed byte stream, ResolveDomain never panics; and any
// stream that decodes to fewer than two records resolves to UnknownDomain.
func TestProperty6_MalformedOriginRobustness(t *testing.T) {
	const iterations = 200
	rng := rand.New(rand.NewSource(0x6d616c66)) // seeded for reproducibility

	for i := 0; i < iterations; i++ {
		switch rng.Intn(3) {
		case 0:
			// Arbitrary random bytes: must never panic. ResolveDomain is
			// expected to return some string without crashing.
			n := rng.Intn(64)
			data := make([]byte, n)
			rng.Read(data)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("iter %d: ResolveDomain panicked on random bytes %v: %v", i, data, r)
					}
				}()
				_ = ResolveDomain(data)
			}()

		case 1:
			// A valid encoding truncated at a random point. Truncating a stream
			// with < 2 complete records must resolve to UnknownDomain, and must
			// never panic.
			scheme := randomText(rng, 1+rng.Intn(6))
			host := randomText(rng, 1+rng.Intn(12))
			full := encodeRecords(scheme, host)
			// Cut somewhere strictly inside the second record so fewer than two
			// records decode cleanly. The first record occupies
			// recordHeaderSize+len(scheme) bytes; cut past that but before the
			// end.
			firstLen := recordHeaderSize + len(scheme)
			if firstLen >= len(full) {
				continue
			}
			cut := firstLen + rng.Intn(len(full)-firstLen) // [firstLen, len-1]
			truncated := full[:cut]

			var got string
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("iter %d: ResolveDomain panicked on truncated stream: %v", i, r)
					}
				}()
				got = ResolveDomain(truncated)
			}()
			if got != UnknownDomain {
				t.Fatalf("iter %d: truncated stream resolved to %q, want %q", i, got, UnknownDomain)
			}

		case 2:
			// Fewer than two decodable records: zero or one well-formed record.
			var texts []string
			if rng.Intn(2) == 0 {
				texts = nil
			} else {
				texts = []string{randomText(rng, rng.Intn(10))}
			}
			data := encodeRecords(texts...)

			var got string
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("iter %d: ResolveDomain panicked on short stream: %v", i, r)
					}
				}()
				got = ResolveDomain(data)
			}()
			if got != UnknownDomain {
				t.Fatalf("iter %d: %d-record stream resolved to %q, want %q", i, len(texts), got, UnknownDomain)
			}
		}
	}
}

// randomText returns a random string of n runes drawn from a mix of ASCII and
// multi-byte UTF-8 code points, so round-trip tests exercise the byte-length
// (not rune-length) prefixing of the encoding.
func randomText(rng *rand.Rand, n int) string {
	if n == 0 {
		return ""
	}
	// Alphabet includes ASCII plus a few multi-byte runes.
	alphabet := []rune("abcdefghijklmnopqrstuvwxyz0123456789.-_/:é üïæ漢字🌐")
	rs := make([]rune, n)
	for i := range rs {
		rs[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(rs)
}
