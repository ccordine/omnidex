package codex

import (
	"context"
	"os"
	"testing"
	"time"
)

// No inference: measure process/initialize and synchronous close independently.
func TestParserLiveStartupShutdown(t *testing.T) {
	command := os.Getenv("OMNI_TEST_LIVE_CODEX")
	if command == "" {
		t.Skip("requires installed native Codex")
	}
	for sample := 1; sample <= 3; sample++ {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		client, err := NewParser(ctx, command, t.TempDir(), 10*time.Second)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		start := time.Now()
		connection, release, err := client.generationConnection(ctx)
		if err != nil {
			cancel()
			_ = client.Close()
			t.Fatal(err)
		}
		_, err = connection.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "omnidex", "version": "0.5.0"}, "capabilities": map[string]bool{"experimentalApi": true}}, nil)
		if err == nil {
			err = connection.send("initialized", map[string]any{}, nil)
		}
		startup := time.Since(start)
		release(true)
		start = time.Now()
		_ = client.Close()
		shutdown := time.Since(start)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("LIFECYCLE sample=%d startup_initialize_ms=%.3f shutdown_ms=%.3f model_calls=0", sample, float64(startup.Microseconds())/1000, float64(shutdown.Microseconds())/1000)
	}
}
