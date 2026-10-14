package tui

import (
	"math/rand"
	"testing"
)

// TestProperty11ConfirmNoCancels drives handleKey(KeyConfirmNo) against random
// modeConfirm Models at random selection indices, asserting that pressing
// 'n'/'N' cancels the pending deletion: the Model returns to modeList with the
// Selection_Index unchanged, and the reported action is actNone — never
// actDelete. This guarantees a negative confirmation can never trigger a
// deletion and never disturbs the operator's current selection.
//
// Feature: interactive-tui-mode, Property 11: n/N cancels without deleting and preserves the index
//
// Validates: Requirements 5.4
func TestProperty11ConfirmNoCancels(t *testing.T) {
	const iterations = 1000 // well above the 100-iteration minimum
	rng := rand.New(rand.NewSource(11))

	for iter := 0; iter < iterations; iter++ {
		n := rng.Intn(30) + 1 // non-empty list, length 1..30
		i := rng.Intn(n)      // selection index in [0, n)

		m := Model{
			folders: makeFolders(n),
			index:   i,
			mode:    modeConfirm,
		}

		got, act := m.handleKey(KeyConfirmNo)

		// The cancel must never request a deletion.
		if act == actDelete {
			t.Fatalf("iter %d: n=%d i=%d: handleKey(KeyConfirmNo) returned actDelete, want actNone",
				iter, n, i)
		}
		if act != actNone {
			t.Fatalf("iter %d: n=%d i=%d: handleKey(KeyConfirmNo) action = %v, want actNone",
				iter, n, i, act)
		}

		// Cancelling returns to the list.
		if got.mode != modeList {
			t.Fatalf("iter %d: n=%d i=%d: handleKey(KeyConfirmNo) mode = %v, want modeList",
				iter, n, i, got.mode)
		}

		// The Selection_Index is preserved.
		if got.index != i {
			t.Fatalf("iter %d: n=%d: handleKey(KeyConfirmNo) index = %d, want %d (unchanged)",
				iter, n, got.index, i)
		}
	}
}
