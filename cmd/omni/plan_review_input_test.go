package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestPlanReviewInputRouterRequiresSource(t *testing.T) {
	t.Parallel()

	router, err := newPlanReviewInputRouter(nil)
	if err == nil || router != nil {
		t.Fatalf("newPlanReviewInputRouter(nil) = (%v, %v), want nil router and error", router, err)
	}
}

func TestPlanReviewInputRouterPassesOrdinaryInputThroughExactly(t *testing.T) {
	t.Parallel()

	want := []byte("first line\r\x1b[Asecond\n")
	router := mustPlanReviewInputRouter(t, strings.NewReader(string(want)))
	got, err := io.ReadAll(router)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("ReadAll() = %q, want %q", got, want)
	}
}

func TestPlanReviewInputRouterDecodesReviewKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  planReviewKey
	}{
		{name: "CSI up", input: "\x1b[A", want: planReviewKeyUp},
		{name: "CSI down", input: "\x1b[B", want: planReviewKeyDown},
		{name: "SS3 up", input: "\x1bOA", want: planReviewKeyUp},
		{name: "SS3 down", input: "\x1bOB", want: planReviewKeyDown},
		{name: "space", input: " ", want: planReviewKeyToggle},
		{name: "carriage return", input: "\r", want: planReviewKeyEnter},
		{name: "line feed", input: "\n", want: planReviewKeyEnter},
		{name: "escape", input: "\x1b", want: planReviewKeyEscape},
		{name: "Ctrl-D", input: "\x04", want: planReviewKeyEOF},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			router := mustPlanReviewInputRouter(t, strings.NewReader(test.input))
			router.EnableReview()
			if got := nextPlanReviewKey(t, router); got != test.want {
				t.Fatalf("NextReviewKey() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestPlanReviewInputRouterCtrlDClosesReviewWithoutLeakingInput(t *testing.T) {
	t.Parallel()

	router := mustPlanReviewInputRouter(t, strings.NewReader("\x04"))
	router.EnableReview()
	if got := nextPlanReviewKey(t, router); got != planReviewKeyEOF {
		t.Fatalf("NextReviewKey() = %d, want EOF", got)
	}
	if router.ReviewEnabled() {
		t.Fatal("ReviewEnabled() = true after Ctrl-D")
	}
	got, err := io.ReadAll(router)
	if err != nil || len(got) != 0 {
		t.Fatalf("ReadAll() after Ctrl-D = %q, %v; want no leaked input", got, err)
	}
}

func TestPlanReviewInputRouterCollapsesCRLFToOneEnter(t *testing.T) {
	t.Parallel()

	router := mustPlanReviewInputRouter(t, strings.NewReader("\r\n"))
	router.EnableReview()
	if got := nextPlanReviewKey(t, router); got != planReviewKeyEnter {
		t.Fatalf("NextReviewKey() = %d, want Enter", got)
	}
	_, err := router.NextReviewKey(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("second NextReviewKey() error = %v, want EOF", err)
	}
}

func TestPlanReviewInputRouterHoldsNoteUntilSessionAcceptsTransition(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"n", "N"} {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			router := mustPlanReviewInputRouter(t, strings.NewReader(prefix+"write the note\r"))
			router.EnableReview()
			if got := nextPlanReviewKey(t, router); got != planReviewKeyNote {
				t.Fatalf("NextReviewKey() = %d, want Note", got)
			}
			if !router.ReviewEnabled() {
				t.Fatal("ReviewEnabled() = false before the session accepted Note")
			}
			readDone := beginPlanReviewRead(router)
			assertPlanReviewReadBlocked(t, readDone)
			if _, err := router.BeginNoteInput(); err != nil {
				t.Fatalf("BeginNoteInput() error = %v", err)
			}
			result := awaitPlanReviewRead(t, readDone)
			if result.err != nil {
				t.Fatalf("Read() error = %v", result.err)
			}
			if string(result.data) != "write the note\r" {
				t.Fatalf("Read() = %q, want same-read note body", result.data)
			}
		})
	}
}

func TestPlanReviewInputRouterConsumesUnknownAndPartialReviewInput(t *testing.T) {
	t.Parallel()

	input := "x\x03\x7f\x1b[C\x1bOD\x1b["
	router := mustPlanReviewInputRouter(t, strings.NewReader(input))
	router.EnableReview()
	got, err := io.ReadAll(router)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ReadAll() leaked review bytes %q", got)
	}
	_, err = router.NextReviewKey(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("NextReviewKey() error = %v, want EOF with no decoded key", err)
	}
}

func TestPlanReviewInputRouterDisableClearsQueuedReviewKeys(t *testing.T) {
	t.Parallel()

	router := mustPlanReviewInputRouter(t, strings.NewReader("  "))
	router.EnableReview()
	if got := nextPlanReviewKey(t, router); got != planReviewKeyToggle {
		t.Fatalf("NextReviewKey() = %d, want Toggle", got)
	}
	router.DisableReview()
	_, err := router.NextReviewKey(context.Background())
	if !errors.Is(err, errPlanReviewInputInactive) {
		t.Fatalf("NextReviewKey() error = %v, want inactive", err)
	}
}

func TestPlanReviewInputRouterPreservesDataBeforeSourceError(t *testing.T) {
	t.Parallel()

	router := mustPlanReviewInputRouter(t, &planReviewDataErrorReader{
		data: []byte("retained"),
		err:  io.ErrUnexpectedEOF,
	})
	got, err := io.ReadAll(router)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadAll() error = %v, want unexpected EOF", err)
	}
	if string(got) != "retained" {
		t.Fatalf("ReadAll() = %q, want retained data", got)
	}
}

func TestPlanReviewInputRouterNextKeyRequiresActiveModeAndContext(t *testing.T) {
	t.Parallel()

	router := mustPlanReviewInputRouter(t, strings.NewReader(""))
	_, err := router.NextReviewKey(context.Background())
	if !errors.Is(err, errPlanReviewInputInactive) {
		t.Fatalf("NextReviewKey() error = %v, want inactive", err)
	}
	router.EnableReview()
	_, err = router.NextReviewKey(nil)
	if err == nil {
		t.Fatal("NextReviewKey(nil) error = nil")
	}
}

func mustPlanReviewInputRouter(t *testing.T, source io.Reader) *planReviewInputRouter {
	t.Helper()
	router, err := newPlanReviewInputRouter(newPlanReviewTestSource(source))
	if err != nil {
		t.Fatalf("newPlanReviewInputRouter() error = %v", err)
	}
	return router
}

func nextPlanReviewKey(t *testing.T, router *planReviewInputRouter) planReviewKey {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	key, err := router.NextReviewKey(ctx)
	if err != nil {
		t.Fatalf("NextReviewKey() error = %v", err)
	}
	return key
}

type planReviewDataErrorReader struct {
	data []byte
	err  error
	done bool
}

func (reader *planReviewDataErrorReader) Read(destination []byte) (int, error) {
	if reader.done {
		return 0, reader.err
	}
	reader.done = true
	return copy(destination, reader.data), reader.err
}
