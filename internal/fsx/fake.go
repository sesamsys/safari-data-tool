package fsx

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// FakeFS is a temp-dir-backed FileSystem implementation for use in tests. It
// roots every operation under a real temporary directory so that WalkDir, Open,
// Stat, ReadDir, and RemoveAll keep the exact same semantics as the production
// osFS, while never touching the TCC-protected live Safari container.
//
// FakeFS is intentionally kept in a non-_test.go file so it can be imported by
// the tests of every package in the module to build fixtures easily.
type FakeFS struct {
	// root is the absolute path of the temp directory backing this fake tree.
	root string
}

// Compile-time assertion that *FakeFS satisfies the FileSystem interface.
var _ FileSystem = (*FakeFS)(nil)

// NewFakeFS returns a FakeFS rooted at the given directory. The directory is
// created (along with any missing parents) if it does not already exist. Tests
// typically pass t.TempDir() so cleanup is handled automatically:
//
//	fake := fsx.NewFakeFS(t.TempDir())
func NewFakeFS(root string) *FakeFS {
	// Best-effort creation of the root; callers normally pass an existing
	// t.TempDir(), but creating it here keeps the constructor convenient.
	_ = os.MkdirAll(root, 0o755)
	return &FakeFS{root: root}
}

// Root returns the absolute path of the directory backing the fake tree. Tests
// use it to construct absolute paths to seeded files and folders.
func (f *FakeFS) Root() string { return f.root }

// Stat returns the FileInfo for the named path.
func (f *FakeFS) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

// ReadDir reads the named directory, returning its entries sorted by filename.
func (f *FakeFS) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }

// Open opens the named file for reading as an io.ReadCloser.
func (f *FakeFS) Open(path string) (io.ReadCloser, error) { return os.Open(path) }

// WalkDir walks the file tree rooted at root, calling fn for each file or
// directory in the tree, including root.
func (f *FakeFS) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}

// RemoveAll removes path and any children it contains.
func (f *FakeFS) RemoveAll(path string) error { return os.RemoveAll(path) }

// Mkdir creates the directory at the given path relative to the fake root
// (along with any missing parents) and returns its absolute path. It is a
// fixture helper for building a fake tree in tests.
func (f *FakeFS) Mkdir(rel string) (string, error) {
	abs := filepath.Join(f.root, rel)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return abs, nil
}

// WriteFile writes data to the file at the given path relative to the fake
// root, creating any missing parent directories, and returns its absolute
// path. It is a fixture helper for building a fake tree in tests.
func (f *FakeFS) WriteFile(rel string, data []byte) (string, error) {
	abs := filepath.Join(f.root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return "", err
	}
	return abs, nil
}

// Path returns the absolute path for a path relative to the fake root without
// creating anything. It is a convenience for assertions and for constructing
// targets to pass back into FileSystem operations.
func (f *FakeFS) Path(rel string) string { return filepath.Join(f.root, rel) }
