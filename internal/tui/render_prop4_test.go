package tui

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// prop4RandomFolders builds a random, non-empty slice of ScannedFolder values
// so the list/confirm modes always have a selectable item to render.
func prop4RandomFolders(rng *rand.Rand) []model.ScannedFolder {
	n := rng.Intn(20) + 1 // 1..20 folders (always non-empty)
	folders := make([]model.ScannedFolder, n)
	for i := range folders {
		id := prop4Itoa(i)
		folders[i] = model.ScannedFolder{
			Hash:    "hash" + id,
			AbsPath: "/base/hash" + id,
			SizeKB:  int64(rng.Intn(1000)),
			Domain:  "example" + id + ".com",
		}
	}
	return folders
}

func prop4Itoa(i int) string {
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

// Feature: interactive-tui-mode, Property 4: A context-appropriate legend is present on every frame
//
// For any Model, the frame produced by render contains a Key_Legend whose
// content matches the current screen: the confirmLegend (confirm/cancel keys)
// in modeConfirm, and the listLegend (move/delete/quit keys) in every other
// mode (list, access, all-clear). Each iteration picks a random mode and builds
// a Model appropriate to it: a non-empty folder list with a valid index for
// list/confirm, a random non-OK AccessStatus for access, and the all-clear
// state.
//
// **Validates: Requirements 2.6, 5.6, 9.4**
func TestRender_Property4_LegendAlwaysPresent(t *testing.T) {
	// nonOKStatuses are the non-OK access statuses modeAccess renders guidance
	// for; render picks ui.AccessMessage(status) for whichever is chosen.
	nonOKStatuses := []model.AccessStatus{
		model.StatusAccessBlocked,
		model.StatusNotFound,
	}

	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Pick a random mode among the four render paths.
		modes := []mode{modeList, modeConfirm, modeAccess, modeAllClear}
		chosen := modes[rng.Intn(len(modes))]

		var m Model
		switch chosen {
		case modeList, modeConfirm:
			// Non-empty folders with a valid index: modeConfirm's render path
			// dereferences folders[index], so both must be well-formed.
			folders := prop4RandomFolders(rng)
			m = Model{
				folders: folders,
				index:   rng.Intn(len(folders)), // in [0, n)
				mode:    chosen,
			}
		case modeAccess:
			// A random non-OK status drives the access guidance frame.
			m = Model{
				mode:   modeAccess,
				status: nonOKStatuses[rng.Intn(len(nonOKStatuses))],
			}
		case modeAllClear:
			m = Model{mode: modeAllClear}
		}

		// The legend shown is context-sensitive: confirm mode uses the
		// confirm/cancel legend, every other mode uses the list legend.
		wantLegend := listLegend
		if chosen == modeConfirm {
			wantLegend = confirmLegend
		}

		frame := string(render(m))
		if !strings.Contains(frame, wantLegend) {
			t.Errorf("render frame missing legend %q in mode %v (seed=%d):\n%q",
				wantLegend, chosen, seed, frame)
			return false
		}
		return true
	}

	// Minimum 100 iterations.
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}
