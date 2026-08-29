package queue

import "testing"

func TestPostgresChatMemoryPagesUseLimitPlusOneAndStableOffsets(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	scope := createMemoryScopeForTest(t, repository)
	for index := 0; index < 5; index++ {
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO memory_chunks(project_id,channel_id,source,kind,content,created_at)
			VALUES ($1,$2,'chat-page','reference',$3,NOW()+($4::int * INTERVAL '1 second'))
		`, scope.ProjectID, scope.ChannelID, "memory-page-item", index); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO memory_candidates(project_id,channel_id,candidate_kind,content,confidence,status)
			VALUES ($1,$2,'reference',$3,$4,'candidate')
		`, scope.ProjectID, scope.ChannelID, "candidate-page-item", float64(5-index)/10); err != nil {
			t.Fatal(err)
		}
	}
	firstMemory, err := repository.ListMemoryChunkPage(ctx, "reference", nil, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !firstMemory.HasMore || firstMemory.NextOffset == nil {
		t.Fatalf("first memory page lacks its exact lookahead cursor: %+v", firstMemory)
	}
	secondMemory, err := repository.ListMemoryChunkPage(ctx, "reference", nil, 2, *firstMemory.NextOffset)
	if err != nil {
		t.Fatal(err)
	}
	if !firstMemory.HasMore || firstMemory.NextOffset == nil || *firstMemory.NextOffset != 2 ||
		!secondMemory.HasMore || secondMemory.NextOffset == nil || *secondMemory.NextOffset != 4 ||
		len(firstMemory.Items) != 2 || len(secondMemory.Items) != 2 ||
		firstMemory.Items[1].ID == secondMemory.Items[0].ID {
		t.Fatalf("memory pages first=%+v second=%+v", firstMemory, secondMemory)
	}
	lastMemory, err := repository.ListMemoryChunkPage(ctx, "reference", nil, 2, *secondMemory.NextOffset)
	if err != nil || lastMemory.HasMore || lastMemory.NextOffset != nil || len(lastMemory.Items) != 1 {
		t.Fatalf("last memory page=%+v err=%v", lastMemory, err)
	}

	firstCandidate, err := repository.ListHistoricalMemoryCandidatePage(ctx, 0, "candidate", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !firstCandidate.HasMore || firstCandidate.NextOffset == nil {
		t.Fatalf("first candidate page lacks its exact lookahead cursor: %+v", firstCandidate)
	}
	secondCandidate, err := repository.ListHistoricalMemoryCandidatePage(
		ctx, 0, "candidate", 2, *firstCandidate.NextOffset,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !firstCandidate.HasMore || firstCandidate.NextOffset == nil || *firstCandidate.NextOffset != 2 ||
		!secondCandidate.HasMore || secondCandidate.NextOffset == nil || *secondCandidate.NextOffset != 4 ||
		len(firstCandidate.Items) != 2 || len(secondCandidate.Items) != 2 ||
		firstCandidate.Items[1].ID == secondCandidate.Items[0].ID {
		t.Fatalf("candidate pages first=%+v second=%+v", firstCandidate, secondCandidate)
	}
}
