package tui

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// prop8RandomNonEmptyFolders builds a random, non-empty slice of ScannedFolder
// values so the generated Model always has a selectable item.
func prop8RandomNonEmptyFolders(rng *rand.Rand) []model.ScannedFolder {
	n := rng.Intn(20) + 1 // 1..20 folders (always non-empty)
	folders := make([]model.ScannedFolder, n)
	for i := range folders {
		h := prop8Itoa(i)
		folders[i] = model.ScannedFolder{
			Hash:    "hash" + h,
			AbsPath: "/base/hash" + h,
			SizeKB:  int64(rng.Intn(1000)),
			Domain:  "example" + h + ".com",
		}
	}
	return folders
}

func prop8Itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// Feature: interactive-tui-mode, Property 8: d/D opens confirmation for the selected item
//
// For any Model in modeList with a non-empty Folder_List, applying a KeyDelete
// keypress ('d' or 'D') produces a Model in modeConfirm whose confirmation
// targets the folder at the current Selection_Index (index unchanged, mode ==
// modeConfirm, action == actNone).
//
// **Validates: Requirements 5.1**
func TestHandleKey_Property8_DeleteOpensConfirm(t *testing.T) {
	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		folders := prop8RandomNonEmptyFolders(rng)
		startIndex := rng.Intn(len(folders)) // in [0, n)

		m := Model{
			folders: folders,
			index:   startIndex,
			mode:    modeList,
		}

		got, act := m.handleKey(KeyDelete)

		// The loop performs no side effect; opening the prompt is a pure
		// model transition.
		if act != actNone {
			t.Errorf("handleKey(KeyDelete) action = %v, want actNone (seed=%d)", act, seed)
			return false
		}

		// 'd'/'D' opens the confirmation prompt (Req 5.1).
		if got.mode != modeConfirm {
			t.Errorf("handleKey(KeyDelete) mode = %v, want modeConfirm (seed=%d)", got.mode, seed)
			return false
		}

		// The confirmation targets the folder at the current selection index:
		// the index is unchanged and still points at the same folder.
		if got.index != startIndex {
			t.Errorf("handleKey(KeyDelete) index = %d, want %d (seed=%d)", got.index, startIndex, seed)
			return false
		}
		target := got.folders[got.index]
		want := folders[startIndex]
		if target.AbsPath != want.AbsPath || target.Hash != want.Hash {
			t.Errorf("handleKey(KeyDelete) targeted folder changed: got %+v, want %+v (seed=%d)",
				target, want, seed)
			return false
		}

		return true
	}

	// Minimum 100 iterations.
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}
