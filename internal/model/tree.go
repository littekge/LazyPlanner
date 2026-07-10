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

	nodes := make(map[string]*TodoNode, len(filtered))
	for _, t := range filtered {
		// First occurrence of a UID wins; duplicates are ignored.
		if _, ok := nodes[t.UID]; !ok || t.UID == "" {
			nodes[t.UID] = &TodoNode{Todo: t}
		}
	}

	var roots []*TodoNode
	for _, t := range filtered {
		n := nodes[t.UID]
		parent, ok := nodes[t.ParentUID]
		if ok && t.ParentUID != "" && t.ParentUID != t.UID && !descends(parent, n) {
			parent.Children = append(parent.Children, n)
		} else {
			roots = append(roots, n)
		}
	}

	sortForest(roots)
	return roots
}

// descends reports whether target is node or appears within node's subtree. It
// guards BuildTree against attaching a node into a cycle. The seen set makes it
// safe against malformed data that already formed a cycle in the partial graph
// (e.g. B←→C plus a third child of B), which an unguarded walk would recurse
// through forever and crash on.
func descends(node, target *TodoNode) bool {
	return descendsSeen(node, target, map[*TodoNode]bool{})
}

func descendsSeen(node, target *TodoNode, seen map[*TodoNode]bool) bool {
	if node == target {
		return true
	}
	if seen[node] {
		return false
	}
	seen[node] = true
	for _, c := range node.Children {
		if descendsSeen(c, target, seen) {
			return true
		}
	}
	return false
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
