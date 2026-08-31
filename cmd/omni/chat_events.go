package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

type streamUpdate struct {
	Event client.RealtimeEvent
	Err   error
}

func followJobEvents(
	ctx context.Context,
	apiClient *client.Client,
	initial *client.JobEventStream,
) <-chan streamUpdate {
	updates := make(chan streamUpdate, 64)
	go func() {
		defer close(updates)
		stream := initial
		var lastID uint64
		for {
			event, err := stream.Read()
			if err == nil {
				if event.ID > lastID {
					lastID = event.ID
				}
				if !sendStreamUpdate(ctx, updates, streamUpdate{Event: event}) {
					stream.Close()
					return
				}
				continue
			}
			stream.Close()
			if ctx.Err() != nil {
				return
			}
			if !sendStreamUpdate(ctx, updates, streamUpdate{Err: fmt.Errorf("realtime stream disconnected: %w", err)}) {
				return
			}
			for {
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				stream, err = apiClient.OpenJobEvents(ctx, lastID)
				if err == nil {
					break
				}
				if !sendStreamUpdate(ctx, updates, streamUpdate{Err: fmt.Errorf("realtime reconnect failed: %w", err)}) {
					return
				}
			}
		}
	}()
	return updates
}

func sendStreamUpdate(ctx context.Context, updates chan<- streamUpdate, update streamUpdate) bool {
	select {
	case <-ctx.Done():
		return false
	case updates <- update:
		return true
	}
}
