// Package fsx provides the filesystem abstraction used throughout the tool so
// that every higher layer can be exercised against temp-dir or in-memory
// implementations without touching the TCC-protected Safari container.
//
// The FileSystem interface and the production osFS implementation live here;
// test/fake implementations live in fake.go.
package fsx

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// FileSystem abstracts the disk operations the tool performs so the
// TCC-protected live container can be mocked in tests.
type FileSystem interface {
	Stat(path string) (os.FileInfo, error)
	ReadDir(path string) ([]os.DirEntry, error)
	Open(path string) (io.ReadCloser, error)
	WalkDir(root string, fn fs.WalkDirFunc) error
	RemoveAll(path string) error
}

// osFS is the production FileSystem implementation. It delegates directly to
// the os and filepath packages, performing real reads against the live Safari
// container (which requires Full Disk Access).
type osFS struct{}

// OS returns the production FileSystem backed by the real operating-system
// filesystem. Higher layers obtain the live implementation through this
// constructor so they can be wired against fakes in tests instead.
func OS() FileSystem { return osFS{} }

// Stat returns the FileInfo for the named path.
func (osFS) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

// ReadDir reads the named directory, returning its entries sorted by filename.
func (osFS) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }

// Open opens the named file for reading as an io.ReadCloser.
func (osFS) Open(path string) (io.ReadCloser, error) { return os.Open(path) }

// WalkDir walks the file tree rooted at root, calling fn for each file or
// directory in the tree, including root.
func (osFS) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}

// RemoveAll removes path and any children it contains.
func (osFS) RemoveAll(path string) error { return os.RemoveAll(path) }
