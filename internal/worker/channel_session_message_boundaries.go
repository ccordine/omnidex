package worker

import (
	"context"
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func (runtime *nativeRuntimeV3) consumeSessionMessage(ctx context.Context) (bool, error) {
	if runtime.sessionMessages == nil {
		return false, nil
	}
	return runtime.sessionMessages.consume(ctx)
}

func (runtime *nativeRuntimeV3) afterAcceptedPortableResult(
	ctx context.Context,
	finalize func(assemblyline.PortableJob, assemblyline.PortableResult, error) error,
) func(assemblyline.PortableJob, assemblyline.PortableResult, error) error {
	if runtime.sessionMessages == nil {
		return finalize
	}
	return func(job assemblyline.PortableJob, result assemblyline.PortableResult, validationErr error) error {
		if err := finalize(job, result, validationErr); err != nil {
			return err
		}
		if validationErr != nil {
			return nil
		}
		_, err := runtime.consumeSessionMessage(ctx)
		return err
	}
}

func (runtime *nativeRuntimeV3) transitionAfterSessionMessages(ctx context.Context, commit func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.sessionMessages == nil {
		return commit()
	}
	for {
		if _, err := runtime.consumeSessionMessage(ctx); err != nil {
			return err
		}
		err := commit()
		if !errors.Is(err, queue.ErrPendingChannelSessionMessage) {
			return err
		}
		// Intake racing with a terminal or waiting transition remains durable.
		// Retry that same transition after resolving the next exact message.
	}
}

func (runtime *nativeRuntimeV3) runWithSessionMessages(ctx context.Context, run func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.sessionMessages == nil {
		return run(ctx)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(runCtx) }()
	stopAndJoin := func(err error) error {
		cancel()
		return errors.Join(err, <-done)
	}
	if _, err := runtime.consumeSessionMessage(runCtx); err != nil {
		return stopAndJoin(err)
	}
	ticker := time.NewTicker(stepControlPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return stopAndJoin(ctx.Err())
		case err := <-done:
			if err != nil {
				return err
			}
			if cancelErr := ctx.Err(); cancelErr != nil {
				return cancelErr
			}
			for {
				consumed, consumeErr := runtime.consumeSessionMessage(runCtx)
				if consumeErr != nil {
					return consumeErr
				}
				if !consumed {
					return err
				}
			}
		case <-ticker.C:
			if _, err := runtime.consumeSessionMessage(runCtx); err != nil {
				return stopAndJoin(err)
			}
		}
	}
}
