package main

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

type planReviewTestSourceResult struct {
	data []byte
	err  error
}

type planReviewTestSource struct {
	gate    sync.Mutex
	results <-chan planReviewTestSourceResult
}

func newPlanReviewTestSource(source io.Reader) *planReviewTestSource {
	results := make(chan planReviewTestSourceResult, 1)
	go func() {
		defer close(results)
		buffer := make([]byte, 64)
		for {
			count, err := source.Read(buffer)
			if count > 0 || err != nil {
				results <- planReviewTestSourceResult{
					data: append([]byte(nil), buffer[:count]...), err: err,
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return &planReviewTestSource{results: results}
}

func (source *planReviewTestSource) LockInput() {
	source.gate.Lock()
}

func (source *planReviewTestSource) UnlockInput() {
	source.gate.Unlock()
}

func (source *planReviewTestSource) Read(destination []byte) (int, error) {
	source.LockInput()
	defer source.UnlockInput()
	return source.ReadInputLocked(destination)
}

func (source *planReviewTestSource) ReadInputLocked(destination []byte) (int, error) {
	select {
	case result, ok := <-source.results:
		if !ok {
			return 0, io.EOF
		}
		return copy(destination, result.data), result.err
	case <-time.After(time.Millisecond):
		return 0, errPlanReviewInputSourceIdle
	}
}

type planReviewReadResult struct {
	data []byte
	err  error
}

func waitForPlanReviewBufferedInput(
	t *testing.T,
	router *planReviewInputRouter,
	review bool,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		router.mu.Lock()
		buffered := len(router.lineBytes)
		if review {
			buffered = len(router.reviewKeys)
		}
		router.mu.Unlock()
		if buffered > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("input router did not buffer source input")
}

func beginPlanReviewRead(router io.Reader) <-chan planReviewReadResult {
	done := make(chan planReviewReadResult, 1)
	go func() {
		buffer := make([]byte, 64)
		count, err := router.Read(buffer)
		done <- planReviewReadResult{data: append([]byte(nil), buffer[:count]...), err: err}
	}()
	return done
}

func assertPlanReviewReadBlocked(t *testing.T, done <-chan planReviewReadResult) {
	t.Helper()
	select {
	case result := <-done:
		t.Fatalf("Read() returned early with (%q, %v)", result.data, result.err)
	case <-time.After(2 * planReviewEscapeTimeout):
	}
}

func awaitPlanReviewRead(t *testing.T, done <-chan planReviewReadResult) planReviewReadResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(time.Second):
		t.Fatal("Read() remained blocked")
		return planReviewReadResult{}
	}
}

func writePlanReviewInput(t *testing.T, writer *io.PipeWriter, value string) {
	t.Helper()
	count, err := writer.Write([]byte(value))
	if err != nil {
		t.Fatalf("Write(%q) error = %v", value, err)
	}
	if count != len(value) {
		t.Fatalf("Write(%q) count = %d, want %d", value, count, len(value))
	}
}

func waitForPlanReviewDecoderState(
	t *testing.T,
	router *planReviewInputRouter,
	want planReviewDecoderState,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		router.mu.Lock()
		got := router.decoderState
		router.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("decoder did not reach state %d", want)
}

type planReviewContextReader struct {
	ctx context.Context
}

func (reader planReviewContextReader) Read([]byte) (int, error) {
	<-reader.ctx.Done()
	return 0, reader.ctx.Err()
}
