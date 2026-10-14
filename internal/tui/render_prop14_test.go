package tui

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
)

// Feature: interactive-tui-mode, Property 14: The confirmation prompt is the last line, with the legend above it
//
// For any Model in modeConfirm over a non-empty Folder_List, the rendered frame
// contains the confirmLegend on a line that precedes the "Proceed? [y/N]:"
// prompt, and the prompt is the final content of the frame with no trailing
// line ending after it — so the operator's keypress appears directly after the
// prompt rather than on a legend line below it.
//
// **Validates: Requirements 5.6, 5.7**
func TestRender_Property14_ConfirmPromptIsLastLine(t *testing.T) {
	const promptTail = "Proceed? [y/N]: "

	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		folders := prop4RandomFolders(rng)
		index := rng.Intn(len(folders))

		m := Model{
			folders: folders,
			index:   index,
			mode:    modeConfirm,
		}

		frame := string(render(m))

		// The prompt is the final content of the frame (no trailing newline
		// after it, so the cursor rests on the prompt line).
		if !strings.HasSuffix(frame, promptTail) {
			t.Errorf("confirm frame does not end with the prompt (seed=%d):\n%q", seed, frame)
			return false
		}

		// The confirm legend appears, and it appears before the prompt.
		li := strings.Index(frame, confirmLegend)
		if li < 0 {
			t.Errorf("confirm frame missing confirmLegend %q (seed=%d):\n%q", confirmLegend, seed, frame)
			return false
		}
		pi := strings.Index(frame, promptTail)
		if li >= pi {
			t.Errorf("confirmLegend at %d is not above the prompt at %d (seed=%d):\n%q", li, pi, seed, frame)
			return false
		}

		// The generic list legend must NOT appear on the confirmation screen.
		if strings.Contains(frame, listLegend) {
			t.Errorf("confirm frame unexpectedly contains the list legend (seed=%d):\n%q", seed, frame)
			return false
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}
