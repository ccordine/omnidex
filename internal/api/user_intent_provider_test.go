package api

import (
	"context"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/config"
)

func TestUserIntentProviderOwnedByServerLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{lifecycleContext: ctx, providerConfig: config.Config{CodexCommand: "fixture-provider", CodexDirectory: t.TempDir()}}
	const count = 8
	providers := make(chan *codex.Client, count)
	failures := make(chan error, count)
	var group sync.WaitGroup
	for i := 0; i < count; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			provider, err := server.userIntentProvider()
			providers <- provider
			failures <- err
		}()
	}
	group.Wait()
	close(providers)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for provider := range providers {
		if provider == nil || provider != server.userParser {
			t.Fatal("interpretations did not share the server provider")
		}
	}
	server.CloseUserIntentProvider()
}
