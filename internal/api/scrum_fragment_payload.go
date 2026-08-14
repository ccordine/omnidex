package api

import "fmt"

type scrumBoardFragmentPayload struct {
	ProjectID        int64
	Board            ScrumBoard
	AllColumns       []string
	CardsByColumn    map[string][]ScrumCard
	ColumnCounts     map[string]int
	PlayQueue        map[string]any
	AutoWork         ScrumAutoWorkConfig
	AutoWorkComplete bool
	FlowSummary      ScrumFlowProjectSummary
	VisibleColumn    string
	CardOffset       int
	CardHasMore      bool
}

func scrumBoardFragmentsForPayload(payload map[string]any, fullBoard ScrumBoard) error {
	decoded, err := decodeScrumBoardFragmentPayload(payload)
	if err != nil {
		return err
	}
	fragments, err := renderScrumBoardFragments(decoded.Board, decoded.CardsByColumn, fullBoard, decoded.VisibleColumn, decoded.ColumnCounts, decoded.PlayQueue, decoded.AutoWork.Enabled, decoded.AutoWork, decoded.AutoWorkComplete, decoded.FlowSummary, scrumCardPageState{
		Offset: decoded.CardOffset, Count: len(decoded.Board.Cards), HasMore: decoded.CardHasMore,
	})
	if err != nil {
		return err
	}
	payload["html"] = fragments
	return nil
}

func decodeScrumBoardFragmentPayload(payload map[string]any) (scrumBoardFragmentPayload, error) {
	if payload == nil {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload is required")
	}
	allowed := map[string]struct{}{
		"project_id": {}, "board": {}, "cards_by_col": {}, "play_queue": {},
		"flow_summary": {}, "all_columns": {}, "visible_column": {}, "column_counts": {},
		"card_offset": {}, "card_has_more": {}, "auto_work": {}, "auto_work_complete": {},
	}
	if len(payload) != len(allowed) {
		return scrumBoardFragmentPayload{}, fmt.Errorf(
			"Scrum fragment payload must contain exactly %d typed fields, received %d",
			len(allowed), len(payload),
		)
	}
	for key := range payload {
		if _, ok := allowed[key]; !ok {
			return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload contains unknown field %q", key)
		}
	}
	projectID, ok := payload["project_id"].(int64)
	if !ok || projectID <= 0 {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload project_id has type %T or is not positive, expected int64", payload["project_id"])
	}
	board, ok := payload["board"].(ScrumBoard)
	if !ok {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload board has type %T, expected api.ScrumBoard", payload["board"])
	}
	cardsByColumn, ok := payload["cards_by_col"].(map[string][]ScrumCard)
	if !ok || cardsByColumn == nil {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload cards_by_col has type %T, expected a typed non-nil card map", payload["cards_by_col"])
	}
	allColumns, ok := payload["all_columns"].([]string)
	if !ok || allColumns == nil {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload all_columns has type %T, expected a typed non-nil string list", payload["all_columns"])
	}
	columnCounts, ok := payload["column_counts"].(map[string]int)
	if !ok || columnCounts == nil {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload column_counts has type %T, expected a typed non-nil count map", payload["column_counts"])
	}
	playQueue, ok := payload["play_queue"].(map[string]any)
	if !ok {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload play_queue has type %T, expected a typed object", payload["play_queue"])
	}
	autoWork, ok := payload["auto_work"].(ScrumAutoWorkConfig)
	if !ok {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload auto_work has type %T, expected api.ScrumAutoWorkConfig", payload["auto_work"])
	}
	autoWorkComplete, ok := payload["auto_work_complete"].(bool)
	if !ok {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload auto_work_complete has type %T, expected bool", payload["auto_work_complete"])
	}
	flowSummary, ok := payload["flow_summary"].(ScrumFlowProjectSummary)
	if !ok {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload flow_summary has type %T, expected api.ScrumFlowProjectSummary", payload["flow_summary"])
	}
	visibleColumn, ok := payload["visible_column"].(string)
	if !ok {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload visible_column has type %T, expected string", payload["visible_column"])
	}
	cardOffset, ok := payload["card_offset"].(int)
	if !ok {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload card_offset has type %T, expected int", payload["card_offset"])
	}
	cardHasMore, ok := payload["card_has_more"].(bool)
	if !ok {
		return scrumBoardFragmentPayload{}, fmt.Errorf("Scrum fragment payload card_has_more has type %T, expected bool", payload["card_has_more"])
	}
	return scrumBoardFragmentPayload{
		ProjectID: projectID, Board: board, AllColumns: allColumns,
		CardsByColumn: cardsByColumn, ColumnCounts: columnCounts,
		PlayQueue: playQueue, AutoWork: autoWork, AutoWorkComplete: autoWorkComplete,
		FlowSummary: flowSummary, VisibleColumn: visibleColumn, CardOffset: cardOffset,
		CardHasMore: cardHasMore,
	}, nil
}
