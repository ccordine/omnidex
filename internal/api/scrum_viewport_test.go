package api

import (
	"net/http"
	"testing"
)

func TestScrumViewportColumnDefaultsToAssigned(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/v1/scrum", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := scrumViewportColumn(req, []string{"backlog", "assigned"}); got != "assigned" {
		t.Fatalf("scrumViewportColumn()=%q want assigned", got)
	}
}
