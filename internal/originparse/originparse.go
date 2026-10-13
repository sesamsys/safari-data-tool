// Package originparse decodes Safari's plaintext origin files and resolves a
// hashed folder to its human-readable domain.
//
// An origin file is a sequence of records, each encoded as a 4-byte
// little-endian unsigned length N, followed by one flag byte, followed by N
// bytes of UTF-8 text. Records pair up as (scheme, host); the host of the
// first pair is the primary domain for the hashed folder.
package originparse

import (
	"encoding/binary"
	"errors"
	"io"
	"io/fs"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
)

// UnknownDomain is the placeholder resolved domain used when no origin file can
// be located or parsed for a hashed folder.
const UnknownDomain = "Unknown Domain"

// ErrMalformed is returned by ParseRecords when the record stream is truncated
// or otherwise malformed. The records decoded before the fault are still
// returned alongside the error.
var ErrMalformed = errors.New("originparse: malformed origin record")

// recordHeaderSize is the fixed size of a record's header: a 4-byte
// little-endian length followed by a single flag byte.
const recordHeaderSize = 5

// Record is one decoded origin record.
type Record struct{ Text string }

// ParseRecords decodes the full record sequence from an origin file's bytes.
// It reads [uint32 LE length N][1 flag byte][N UTF-8 bytes] repeatedly until the
// input is exhausted. On a malformed or truncated record it stops and returns
// the records decoded so far plus ErrMalformed, never panicking.
//
// A clean end of data occurs when the cursor reaches exactly the end of the
// input. Fewer than recordHeaderSize bytes remaining mid-stream, or a declared
// length that runs past the end of the input, is treated as truncation.
func ParseRecords(data []byte) ([]Record, error) {
	var records []Record
	cursor := 0
	for cursor < len(data) {
		// Need at least a full header to read the next record. A partial
		// header mid-stream is a truncated record, not a clean end.
		if len(data)-cursor < recordHeaderSize {
			return records, ErrMalformed
		}
		n := binary.LittleEndian.Uint32(data[cursor : cursor+4])
		// Guard N against absurd sizes to avoid huge allocations: a legitimate
		// text cannot be longer than the remaining input.
		if uint64(n) > uint64(len(data)) {
			return records, ErrMalformed
		}
		textStart := cursor + recordHeaderSize
		textEnd := textStart + int(n)
		// Truncated record: the declared text runs past the end of the input.
		if textEnd > len(data) {
			return records, ErrMalformed
		}
		records = append(records, Record{Text: string(data[textStart:textEnd])})
		cursor = textEnd
	}
	return records, nil
}

// ResolveDomain returns the host of the first scheme/host pair: the text of the
// second record. It returns UnknownDomain when fewer than two records decode.
func ResolveDomain(data []byte) string {
	records, _ := ParseRecords(data)
	if len(records) < 2 {
		return UnknownDomain
	}
	return records[1].Text
}

// FindOriginFile walks the hashed folder tree and returns the path of the first
// file named "origin", or "" if none is found.
func FindOriginFile(f fsx.FileSystem, folder string) string {
	var found string
	_ = f.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries and keep walking.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() && d.Name() == "origin" {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// Resolve ties finding + reading + decoding together, returning the resolved
// primary domain and the full list of host texts. Even-indexed records are
// schemes and odd-indexed records are hosts; the hosts are collected into
// hosts. Any failure yields UnknownDomain and a nil host list.
func Resolve(f fsx.FileSystem, folder string) (domain string, hosts []string) {
	path := FindOriginFile(f, folder)
	if path == "" {
		return UnknownDomain, nil
	}
	rc, err := f.Open(path)
	if err != nil {
		return UnknownDomain, nil
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return UnknownDomain, nil
	}
	records, _ := ParseRecords(data)
	if len(records) < 2 {
		return UnknownDomain, nil
	}
	for i := 1; i < len(records); i += 2 {
		hosts = append(hosts, records[i].Text)
	}
	return records[1].Text, hosts
}
