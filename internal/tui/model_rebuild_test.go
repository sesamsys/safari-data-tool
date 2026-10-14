package tui

import (
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// TestRebuildAfterDeleteAllClearDisambiguation pins the all-clear vs. access
// disambiguation in rebuildAfterDelete. scanner.Classify maps an empty base to
// StatusAccessBlocked, so an empty post-deletion re-scan looks identical to a
// genuine startup access block on the status alone. rebuildAfterDelete breaks
// the tie with the prior model's sessionHasLoaded: a loaded session that now
// yields no folders is the All_Clear_State (modeAllClear), while an unloaded
// session stays in modeAccess guidance.
//
// Feature: interactive-tui-mode, example: all-clear disambiguation
//
// Validates: Requirements 6.4, 8.4, 9.2
func TestRebuildAfterDeleteAllClearDisambiguation(t *testing.T) {
	t.Run("loaded session with empty re-scan promotes to modeAllClear", func(t *testing.T) {
		prev := Model{sessionHasLoaded: true}

		rebuilt := rebuildAfterDelete(prev, nil, model.StatusAccessBlocked)

		if rebuilt.mode != modeAllClear {
			t.Errorf("rebuildAfterDelete(sessionHasLoaded=true, empty, StatusAccessBlocked).mode = %d, want modeAllClear (%d)", rebuilt.mode, modeAllClear)
		}
	})

	t.Run("unloaded session with empty re-scan stays in modeAccess", func(t *testing.T) {
		prev := Model{sessionHasLoaded: false}

		rebuilt := rebuildAfterDelete(prev, nil, model.StatusAccessBlocked)

		if rebuilt.mode != modeAccess {
			t.Errorf("rebuildAfterDelete(sessionHasLoaded=false, empty, StatusAccessBlocked).mode = %d, want modeAccess (%d)", rebuilt.mode, modeAccess)
		}
	})
}
