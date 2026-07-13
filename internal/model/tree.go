package model

import (
	"sort"
	"strings"
)

// TodoNode is a task together with its child tasks in the subtask forest.
type TodoNode struct {
	Todo     *Todo
	Children []*TodoNode
}

// BuildTree assembles the subtask forest from a flat set of todos, linking
// children to parents via ParentUID (RELATED-TO). A todo whose parent is not in
// the set becomes a root, so incomplete descendants of a hidden parent still
// surface. Siblings are ordered by the smart-sort rule (see SortTodos).
//
// When includeCompleted is false, completed todos are removed before the tree
// is built. Cyclic parent references (malformed data) are broken: nodes only
// reachable through a cycle are dropped rather than looping forever.
func BuildTree(todos []*Todo, includeCompleted bool) []*TodoNode {
	filtered := todos
	if !includeCompleted {
		filtered = filtered[:0:0]
		for _, t := range todos {
			if !t.Completed() {
				filtered = append(filtered, t)
			}
		}
	}

	// Key nodes by UID for parent resolution; first occurrence of a UID wins.
	// UID-less todos (malformed — RFC 5545 requires UID) are not keyed here: they
	// can't be a parent or child and must not collide on a shared "" slot.
	// parentOf records each kept UID's parent link (from the same first occurrence)
	// for the cycle classification below.
	nodes := make(map[string]*TodoNode, len(filtered))
	parentOf := make(map[string]string, len(filtered))
	for _, t := range filtered {
		if t.UID == "" {
			continue
		}
		if _, ok := nodes[t.UID]; !ok {
			nodes[t.UID] = &TodoNode{Todo: t}
			parentOf[t.UID] = t.ParentUID
		}
	}

	reachesRoot := classifyByAncestry(nodes, parentOf)

	placed := make(map[string]bool, len(nodes))
	var roots []*TodoNode
	for _, t := range filtered {
		if t.UID == "" {
			// Each UID-less todo stands alone as its own root, so all of them
			// surface exactly once instead of aliasing (and overwriting) one node.
			roots = append(roots, &TodoNode{Todo: t})
			continue
		}
		if placed[t.UID] {
			continue // a duplicate UID: its node was already placed by the first occurrence
		}
		placed[t.UID] = true
		if !reachesRoot[t.UID] {
			continue // reachable only through a cycle (malformed) — dropped, not looped
		}
		n := nodes[t.UID]
		if p := parentOf[t.UID]; p != "" && p != t.UID && nodes[p] != nil {
			nodes[p].Children = append(nodes[p].Children, n)
		} else {
			roots = append(roots, n)
		}
	}

	sortForest(roots)
	return roots
}

// classifyByAncestry marks each keyed UID true when following its parent links
// eventually reaches a real root (a node with no in-set parent), and false when
// the chain loops back on itself — a cycle in malformed RELATED-TO data, whose
// nodes are dropped rather than walked forever. Each UID is resolved once and
// memoized (iteratively, so a very deep chain can't overflow the stack), so the
// whole classification is linear in the number of todos — replacing the former
// per-insert subtree walk that made BuildTree quadratic on large lists.
func classifyByAncestry(nodes map[string]*TodoNode, parentOf map[string]string) map[string]bool {
	const (
		unknown = iota
		reaches
		cyclic
	)
	status := make(map[string]int, len(nodes))

	for uid := range nodes {
		if status[uid] != unknown {
			continue
		}
		var path []string
		inPath := make(map[string]bool)
		cur, result := uid, 0
		for {
			if s := status[cur]; s != unknown {
				result = s
				break
			}
			if inPath[cur] {
				result = cyclic // the chain looped before reaching a root
				break
			}
			inPath[cur] = true
			path = append(path, cur)
			p := parentOf[cur]
			if p == "" || p == cur || nodes[p] == nil {
				result = reaches // cur is a real root
				break
			}
			cur = p
		}
		for _, u := range path {
			status[u] = result
		}
	}

	out := make(map[string]bool, len(nodes))
	for uid, s := range status {
		out[uid] = s == reaches
	}
	return out
}

func sortForest(nodes []*TodoNode) {
	sort.SliceStable(nodes, func(i, j int) bool { return lessTodo(nodes[i].Todo, nodes[j].Todo) })
	for _, n := range nodes {
		sortForest(n.Children)
	}
}

// SortTodos orders todos in place by the smart-sort rule: earliest due date
// first (undated last), then higher priority (iCal 1 is highest; 0/undefined
// sorts last), then title case-insensitively. The sort is stable.
func SortTodos(todos []*Todo) {
	sort.SliceStable(todos, func(i, j int) bool { return lessTodo(todos[i], todos[j]) })
}

func lessTodo(a, b *Todo) bool {
	if a.HasDue != b.HasDue {
		return a.HasDue // dated tasks before undated ones
	}
	if a.HasDue && b.HasDue && !a.Due.Equal(b.Due) {
		return a.Due.Before(b.Due)
	}
	if pa, pb := priorityRank(a.Priority), priorityRank(b.Priority); pa != pb {
		return pa < pb
	}
	return strings.ToLower(a.Summary) < strings.ToLower(b.Summary)
}

// priorityRank maps iCal priorities to a sort rank where lower sorts first.
// Undefined (0) becomes 10 so it ranks after every defined priority (1–9).
func priorityRank(p int) int {
	if p == PriorityUndefined {
		return 10
	}
	return p
}
