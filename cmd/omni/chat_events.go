package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type streamUpdate struct {
	Event client.RealtimeEvent
	Err   error
	Fatal bool
}

func followJobEvents(
	ctx context.Context,
	apiClient *client.Client,
	channelID model.ChannelID,
	workspaceIdentity string,
	initial *client.JobEventStream,
) <-chan streamUpdate {
	updates := make(chan streamUpdate, 64)
	go func() {
		defer close(updates)
		stream := initial
		var lastID uint64
		for {
			connected := false
			connectedLatest := uint64(0)
			replayRemaining := 0
			for {
				event, err := stream.Read()
				if err != nil {
					_ = stream.Close()
					if ctx.Err() != nil {
						return
					}
					fatal := client.IsPermanentRealtimeError(err)
					if !sendStreamUpdate(ctx, updates, streamUpdate{
						Err: fmt.Errorf("realtime stream disconnected: %w", err), Fatal: fatal,
					}) || fatal {
						return
					}
					break
				}
				if event.EventName == client.RealtimeConnected {
					if connected {
						_ = stream.Close()
						sendStreamUpdate(ctx, updates, streamUpdate{
							Err:   &client.RealtimeProtocolError{Message: "realtime stream repeated its connection frame"},
							Fatal: true,
						})
						return
					}
					connected = true
					connectedLatest = event.LatestID
					replayRemaining = event.ReplayCount
					if event.SyncRequired || replayRemaining == 0 {
						lastID = connectedLatest
					}
				} else {
					if !connected || event.ID <= lastID {
						_ = stream.Close()
						sendStreamUpdate(ctx, updates, streamUpdate{
							Err:   &client.RealtimeProtocolError{Message: "realtime stream returned an out-of-order event"},
							Fatal: true,
						})
						return
					}
					lastID = event.ID
					if replayRemaining > 0 {
						replayRemaining--
						if replayRemaining == 0 && connectedLatest > lastID {
							lastID = connectedLatest
						}
					}
				}
				if !sendStreamUpdate(ctx, updates, streamUpdate{Event: event}) {
					_ = stream.Close()
					return
				}
			}

			for {
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				var err error
				stream, err = apiClient.OpenJobEvents(
					ctx,
					channelID,
					workspaceIdentity,
					&lastID,
				)
				if err == nil {
					break
				}
				fatal := client.IsPermanentRealtimeError(err)
				if !sendStreamUpdate(ctx, updates, streamUpdate{
					Err: fmt.Errorf("realtime reconnect failed: %w", err), Fatal: fatal,
				}) || fatal {
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
