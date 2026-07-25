package ui

import (
	"testing"
	"time"
)

// TestSelectBulkOpDoesNotLeakCount reproduces the "vim count leaks past a SELECT
// bulk-op key" defect: typing a digit then a bulk-op key while selecting must not
// leave a stale pendingCount that then multiplies the next motion.
//
// Root cause: in globalKeys, `if a.handleSelectKey(ev) == nil { return nil }`
// returns for a handled bulk-op key BEFORE the count-reset (a.pendingCount = 0)
// further down. The bulk op exits SELECT, stranding the user in normal mode with
// a stale count that multiplies the next j/k.
func TestSelectBulkOpDoesNotLeakCount(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	var uids []string
	for _, s := range []string{"A", "B", "C"} {
		uids = append(uids, putTodo(t, a, testCalID(a), "", "task "+s, now, true))
	}
	a.refresh(uids[0])
	a.setFocus(a.tree)
	a.enterSelect()
	if !a.selecting {
		t.Fatal("setup: enterSelect did not enter SELECT")
	}

	// User types "3 " while selecting: count 3, then Space = bulkComplete.
	a.globalKeys(runeKey('3'))
	if a.pendingCount != 3 {
		t.Fatalf("after typing '3' pendingCount = %d, want 3", a.pendingCount)
	}
	a.globalKeys(runeKey(' ')) // bulk complete: handled by handleSelectKey, exits SELECT

	if a.selecting {
		t.Fatal("bulk complete must exit SELECT")
	}
	if a.pendingCount != 0 {
		t.Fatalf("count leaked past the SELECT bulk-op key: pendingCount = %d, want 0 "+
			"(the next j/k would move %d rows instead of 1)", a.pendingCount, a.pendingCount)
	}
}
