package message

import (
	"fmt"

	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

type graph struct {
	root    *pmaster.MessageDetail
	details map[string]*pmaster.MessageDetail
}

// FirstPath follows a message graph using the first listed option at each
// choice. It returns the choice detail IDs and the terminal detail.
func FirstPath(master *pmaster.Message) ([]string, *pmaster.MessageDetail, error) {
	graph, err := buildGraph(master)
	if err != nil {
		return nil, nil, err
	}

	selections := make([]string, 0)
	current := graph.root
	visited := make(map[string]struct{}, len(graph.details))
	for {
		if _, duplicate := visited[current.GetMessageDetailId()]; duplicate {
			return nil, nil, fmt.Errorf("message %q detail graph contains a cycle", master.GetId())
		}
		visited[current.GetMessageDetailId()] = struct{}{}
		nextIDs := current.GetNextMessageDetailIds()
		if len(nextIDs) == 0 {
			return selections, current, nil
		}
		next := graph.details[nextIDs[0]]
		if next.GetChoiceText() != "" {
			selections = append(selections, next.GetMessageDetailId())
		}
		current = next
	}
}

// ExistingHistoryPath validates a finished message history and finds one
// complete route represented by its selections. Extra choices from replayed
// alternate routes are accepted because SaveHistory stores their union.
func ExistingHistoryPath(
	master *pmaster.Message,
	selectionIDs []string,
) (*pmaster.MessageDetail, error) {
	graph, err := buildGraph(master)
	if err != nil {
		return nil, err
	}
	selected := make(map[string]struct{}, len(selectionIDs))
	for _, selectionID := range selectionIDs {
		detail := graph.details[selectionID]
		if detail == nil || detail.GetChoiceText() == "" {
			return nil, fmt.Errorf("message %q selection %q is not a choice", master.GetId(), selectionID)
		}
		if _, duplicate := selected[selectionID]; duplicate {
			return nil, fmt.Errorf("message %q selection %q is duplicated", master.GetId(), selectionID)
		}
		selected[selectionID] = struct{}{}
	}
	terminal, ok := selectedPathTerminal(graph.root, graph.details, selected, make(map[string]struct{}))
	if !ok {
		return nil, fmt.Errorf("message %q selections contain no complete path", master.GetId())
	}
	return terminal, nil
}

func selectedPathTerminal(
	current *pmaster.MessageDetail,
	details map[string]*pmaster.MessageDetail,
	selected map[string]struct{},
	visited map[string]struct{},
) (*pmaster.MessageDetail, bool) {
	id := current.GetMessageDetailId()
	if _, duplicate := visited[id]; duplicate {
		return nil, false
	}
	visited = cloneSet(visited)
	visited[id] = struct{}{}
	nextIDs := current.GetNextMessageDetailIds()
	if len(nextIDs) == 0 {
		return current, true
	}
	for _, nextID := range nextIDs {
		next := details[nextID]
		if next.GetChoiceText() != "" {
			if _, chosen := selected[nextID]; !chosen {
				continue
			}
		} else if len(nextIDs) > 1 {
			continue
		}
		if terminal, ok := selectedPathTerminal(next, details, selected, visited); ok {
			return terminal, true
		}
	}
	return nil, false
}

func buildGraph(master *pmaster.Message) (*graph, error) {
	if master == nil || len(master.GetDetails()) == 0 {
		return nil, fmt.Errorf("message has no detail graph")
	}
	details := make(map[string]*pmaster.MessageDetail, len(master.GetDetails()))
	indegree := make(map[string]int, len(master.GetDetails()))
	for _, detail := range master.GetDetails() {
		id := detail.GetMessageDetailId()
		if id == "" || details[id] != nil {
			return nil, fmt.Errorf("message %q has an invalid detail identity", master.GetId())
		}
		details[id] = detail
		indegree[id] = 0
	}
	for _, detail := range master.GetDetails() {
		for _, nextID := range detail.GetNextMessageDetailIds() {
			if details[nextID] == nil {
				return nil, fmt.Errorf("message %q references missing detail %q", master.GetId(), nextID)
			}
			indegree[nextID]++
		}
	}
	var root *pmaster.MessageDetail
	for id, degree := range indegree {
		if degree != 0 {
			continue
		}
		if root != nil {
			return nil, fmt.Errorf("message %q has multiple roots", master.GetId())
		}
		root = details[id]
	}
	if root == nil {
		return nil, fmt.Errorf("message %q has no root", master.GetId())
	}
	return &graph{root: root, details: details}, nil
}

func cloneSet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source)+1)
	for value := range source {
		result[value] = struct{}{}
	}
	return result
}
