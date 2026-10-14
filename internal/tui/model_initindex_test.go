package tui

import (
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// TestNewModelInitialSelectionIndex verifies that building a Model from a
// non-empty StatusOK scan selects the topmost folder: index is 0 and the model
// opens in the list view. This is the example counterpart to the navigation
// properties, pinning the concrete initial-selection behavior.
//
// Feature: interactive-tui-mode, example: initial selection index
//
// Validates: Requirements 3.1
func TestNewModelInitialSelectionIndex(t *testing.T) {
	folders := makeFolders(5) // non-empty folder list

	m := newModel(folders, model.StatusOK)

	if m.index != 0 {
		t.Errorf("newModel(non-empty, StatusOK).index = %d, want 0 (topmost folder)", m.index)
	}
	if m.mode != modeList {
		t.Errorf("newModel(non-empty, StatusOK).mode = %d, want modeList (%d)", m.mode, modeList)
	}
}
