package queue

import (
	"fmt"
	"testing"
)

func TestDataSourcePagesUseDatabaseLimitAndStableOffsets(t *testing.T) {
	ctx, repository := relationalDataSourceTestRepository(t)
	records := make([]DataSourceRecord, 0, 5)
	for index := 0; index < 5; index++ {
		record, err := repository.CreateDataSource(ctx, DataSourceUpsert{
			Name: fmt.Sprintf("Source %d", index), Driver: "postgres",
			Host: "localhost", Port: 5432, DatabaseName: "fixture", Username: "reader",
			SSLMode: "prefer",
		})
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	first, err := repository.ListDataSourcesPage(ctx, DataSourcePageRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ListDataSourcesPage(ctx, DataSourcePageRequest{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	last, err := repository.ListDataSourcesPage(ctx, DataSourcePageRequest{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || !first.HasMore || first.Items[0].ID != records[0].ID ||
		len(second.Items) != 2 || !second.HasMore || second.Items[0].ID != records[2].ID ||
		len(last.Items) != 1 || last.HasMore || last.Items[0].ID != records[4].ID {
		t.Fatalf("unexpected data-source pages first=%+v second=%+v last=%+v", first, second, last)
	}
	exact, err := repository.GetDataSource(ctx, records[4].ID)
	if err != nil || exact.ID != records[4].ID {
		t.Fatalf("exact source=%+v err=%v", exact, err)
	}
}

func TestDataSourceChannelPagesUseDatabaseLimitAndStableOffsets(t *testing.T) {
	ctx, repository := relationalDataSourceTestRepository(t)
	source, err := repository.CreateDataSource(ctx, DataSourceUpsert{
		Name: "Channel source", Driver: "postgres", Host: "localhost", Port: 5432,
		DatabaseName: "fixture", Username: "reader", SSLMode: "prefer",
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 5; index++ {
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO data_source_channels(id,data_source_id,name,created_at,updated_at)
			VALUES($1,$2,$3,NOW(),NOW()+($4::int * INTERVAL '1 second'))
		`, fmt.Sprintf("channel-%d", index), source.ID, fmt.Sprintf("Channel %d", index), index); err != nil {
			t.Fatal(err)
		}
	}
	first, err := repository.ListDataSourceChannelsPage(ctx, source.ID, DataSourcePageRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ListDataSourceChannelsPage(ctx, source.ID, DataSourcePageRequest{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	last, err := repository.ListDataSourceChannelsPage(ctx, source.ID, DataSourcePageRequest{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || !first.HasMore || first.Items[0].ID != "channel-4" ||
		len(second.Items) != 2 || !second.HasMore || second.Items[0].ID != "channel-2" ||
		len(last.Items) != 1 || last.HasMore || last.Items[0].ID != "channel-0" {
		t.Fatalf("unexpected channel pages first=%+v second=%+v last=%+v", first, second, last)
	}
}

func TestDataSourceMessagePagesReadLatestHistoryWithDatabaseBounds(t *testing.T) {
	ctx, repository := relationalDataSourceTestRepository(t)
	source, err := repository.CreateDataSource(ctx, DataSourceUpsert{
		Name: "History source", Driver: "postgres", Host: "localhost", Port: 5432,
		DatabaseName: "fixture", Username: "reader", SSLMode: "prefer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO data_source_channels(id,data_source_id,name,created_at,updated_at)
		VALUES('channel-history',$1,'History',NOW(),NOW())
	`, source.ID); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 5; index++ {
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO data_source_channel_messages(channel_id,role,content,payload,created_at)
			VALUES('channel-history','user',$1,'{}'::jsonb,NOW()+($2::int * INTERVAL '1 second'))
		`, fmt.Sprintf("message-%d", index), index); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := repository.ListDataSourceChannelMessagePage(ctx, "channel-history", DataSourcePageRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	older, err := repository.ListDataSourceChannelMessagePage(ctx, "channel-history", DataSourcePageRequest{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	oldest, err := repository.ListDataSourceChannelMessagePage(ctx, "channel-history", DataSourcePageRequest{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(latest.Items) != 2 || !latest.HasMore || latest.Items[0].Content != "message-3" || latest.Items[1].Content != "message-4" ||
		len(older.Items) != 2 || !older.HasMore || older.Items[0].Content != "message-1" || older.Items[1].Content != "message-2" ||
		len(oldest.Items) != 1 || oldest.HasMore || oldest.Items[0].Content != "message-0" {
		t.Fatalf("unexpected message pages latest=%+v older=%+v oldest=%+v", latest, older, oldest)
	}
}
