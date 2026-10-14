package tui

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
)

// failingFS wraps a real FakeFS but forces RemoveAll to fail, so applyDelete's
// deletion-failure path (Req 5.5) can be exercised. The embedded *fsx.FakeFS
// supplies every other FileSystem method; only RemoveAll is overridden.
type failingFS struct {
	*fsx.FakeFS
}

// RemoveAll always returns a sentinel error so the deletion appears to fail.
func (failingFS) RemoveAll(string) error { return errors.New("boom") }

// TestApplyDeleteRecordsFailure verifies that when deletion fails, applyDelete
// records the error, returns to modeList, and leaves the folder set unchanged
// (Req 5.5).
func TestApplyDeleteRecordsFailure(t *testing.T) {
	// base and target are arranged so deleter.ValidatePath passes (target is a
	// direct child of base), ensuring f.RemoveAll is actually reached.
	base := "/base"
	target := filepath.Join(base, "hash1") // "/base/hash1"

	original := []model.ScannedFolder{
		{Hash: "hash1", AbsPath: target, SizeKB: 100, Domain: "example.com"},
		{Hash: "hash2", AbsPath: filepath.Join(base, "hash2"), SizeKB: 50, Domain: "other.com"},
	}

	// Keep an independent copy to assert the folder set is left unchanged.
	wantFolders := append([]model.ScannedFolder(nil), original...)

	m := Model{
		folders:          original,
		index:            0,
		mode:             modeList,
		status:           model.StatusOK,
		sessionHasLoaded: true,
	}

	fs := failingFS{fsx.NewFakeFS(t.TempDir())}

	got := applyDelete(m, fs, base)

	if got.mode != modeList {
		t.Errorf("mode = %v, want modeList (%v)", got.mode, modeList)
	}
	if got.errMsg == "" {
		t.Error("errMsg is empty, want the recorded deletion failure")
	}
	if !reflect.DeepEqual(got.folders, wantFolders) {
		t.Errorf("folders changed after failed delete:\n got  %+v\n want %+v", got.folders, wantFolders)
	}
}
