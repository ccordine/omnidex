package db

import (
	"context"
	"strings"
	"testing"
)

func TestLegacyPublicCutoverConnectionRejectsURLSearchPathAuthority(t *testing.T) {
	for _, databaseURL := range []string{
		"postgres://user:password@127.0.0.1/database?search_path=other",
		"postgres://user:password@127.0.0.1/database?options=-csearch_path%3Dother",
	} {
		_, err := ConnectLegacyPublicCutover(context.Background(), databaseURL)
		if err == nil || !strings.Contains(err.Error(), "search_path is forbidden") {
			t.Fatalf("ConnectLegacyPublicCutover error=%v", err)
		}
	}
}
