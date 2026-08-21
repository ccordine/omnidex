package worker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type synchronizedContextEmbeddingClient struct {
	started chan string
	release chan struct{}
	vectors map[string][]float64
	errors  map[string]error
}

func (client *synchronizedContextEmbeddingClient) Embedding(
	_ context.Context,
	content string,
) ([]float64, error) {
	client.started <- content
	<-client.release
	return client.vectors[content], client.errors[content]
}

func TestEmbedContextQueriesRunsIndependentEmbeddingsConcurrentlyAndKeepsOrder(t *testing.T) {
	client := &synchronizedContextEmbeddingClient{
		started: make(chan string, 3), release: make(chan struct{}),
		vectors: map[string][]float64{
			"alpha": {1}, "beta": {2}, "gamma": {3},
		},
		errors: map[string]error{},
	}
	type outcome struct {
		vectors [][]float64
		err     error
	}
	completed := make(chan outcome, 1)
	go func() {
		vectors, err := embedContextQueries(
			t.Context(), client, []string{"alpha", "beta", "gamma"},
		)
		completed <- outcome{vectors: vectors, err: err}
	}()

	started := make(map[string]struct{}, 3)
	for len(started) < 3 {
		select {
		case query := <-client.started:
			started[query] = struct{}{}
		case <-time.After(2 * time.Second):
			t.Fatal("embedding calls did not overlap")
		}
	}
	close(client.release)
	result := <-completed
	if result.err != nil {
		t.Fatal(result.err)
	}
	if !reflect.DeepEqual(result.vectors, [][]float64{{1}, {2}, {3}}) {
		t.Fatalf("vectors=%#v", result.vectors)
	}
}

func TestEmbedContextQueriesReportsLowestCanonicalQueryFailure(t *testing.T) {
	first := errors.New("first failure")
	client := &synchronizedContextEmbeddingClient{
		started: make(chan string, 2), release: make(chan struct{}),
		vectors: map[string][]float64{},
		errors:  map[string]error{"alpha": first, "beta": errors.New("second failure")},
	}
	completed := make(chan error, 1)
	go func() {
		_, err := embedContextQueries(t.Context(), client, []string{"alpha", "beta"})
		completed <- err
	}()
	for range 2 {
		select {
		case <-client.started:
		case <-time.After(2 * time.Second):
			t.Fatal("embedding calls did not overlap")
		}
	}
	close(client.release)
	err := <-completed
	if !errors.Is(err, first) || !strings.Contains(err.Error(), "query 0") {
		t.Fatalf("error=%v", err)
	}
}
