package tui

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/ui"
)

// prop7RandomNonEmptyFolders builds a random, non-empty slice of ScannedFolder
// values so the rendered Model always has at least one selectable row.
func prop7RandomNonEmptyFolders(rng *rand.Rand) []model.ScannedFolder {
	n := rng.Intn(20) + 1 // 1..20 folders (always non-empty)
	folders := make([]model.ScannedFolder, n)
	for i := range folders {
		h := prop7Itoa(i)
		folders[i] = model.ScannedFolder{
			Hash:    "hash" + h,
			AbsPath: "/base/hash" + h,
			SizeKB:  int64(rng.Intn(1000)),
			Domain:  "example" + h + ".com",
		}
	}
	return folders
}

func prop7Itoa(i int) string {
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

// Feature: interactive-tui-mode, Property 7: Exactly one inverted row, at the selection index
//
// For any Model in modeList or modeConfirm with a non-empty Folder_List, the
// rendered frame wraps exactly one row in the reverse-video escape (escInvert),
// and that row is the one at the Selection_Index: the frame contains exactly
// one occurrence of escInvert, and that occurrence immediately precedes the
// row text for folders[index], followed by escReset.
//
// **Validates: Requirements 4.1, 4.2**
func TestRender_Property7_ExactlyOneInvertedRow(t *testing.T) {
	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		folders := prop7RandomNonEmptyFolders(rng)
		index := rng.Intn(len(folders)) // in [0, n)

		// modeList and modeConfirm both render folder rows with the selected
		// row inverted; pick one at random to cover both.
		m := Model{
			folders: folders,
			index:   index,
			mode:    modeList,
		}
		if rng.Intn(2) == 0 {
			m.mode = modeConfirm
		}

		frame := string(render(m))

		// Exactly one row is wrapped in reverse video (Req 4.1): there is a
		// single occurrence of the invert escape in the whole frame.
		if got := strings.Count(frame, escInvert); got != 1 {
			t.Errorf("escInvert count = %d, want 1 (seed=%d, mode=%v, index=%d, n=%d)",
				got, seed, m.mode, index, len(folders))
			return false
		}

		// The inverted row is the one at the Selection_Index (Req 4.1): the
		// frame contains escInvert + FormatLine(folders[index]) + escReset.
		wantRow := escInvert + ui.FormatLine(folders[index]) + escReset
		if !strings.Contains(frame, wantRow) {
			t.Errorf("frame does not invert the row at index %d (seed=%d, mode=%v, n=%d)\nwant row=%q",
				index, seed, m.mode, len(folders), wantRow)
			return false
		}

		return true
	}

	// Minimum 100 iterations.
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}
