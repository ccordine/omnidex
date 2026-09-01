package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

var errPlanReviewInputInactive = errors.New("plan review input is not active")
var errPlanReviewInputSourceIdle = errors.New("plan review input source is idle")

// planReviewInputSource serializes one bounded source read with every input
// mode transition. ReadInputLocked must return errPlanReviewInputSourceIdle
// instead of blocking indefinitely when no byte is available.
type planReviewInputSource interface {
	io.Reader
	LockInput()
	UnlockInput()
	ReadInputLocked([]byte) (int, error)
}

type terminalInputMode uint8

const (
	terminalInputOrdinary terminalInputMode = iota
	terminalInputPlanReview
	terminalInputPlanNote
	terminalInputMixed
)

type terminalInputAuthority struct {
	Generation uint64
	Mode       terminalInputMode
}

type planReviewKeyEvent struct {
	Key        planReviewKey
	Generation uint64
}

// planReviewInputRouter is the single reader of source. Outside review mode it
// preserves the source byte stream for the terminal line editor. During review
// mode it consumes every byte and exposes only recognized review keys.
type planReviewInputRouter struct {
	source planReviewInputSource

	started sync.Once
	review  atomic.Bool

	mu                sync.Mutex
	lineBytes         []byte
	lineAuthority     terminalInputAuthority
	noteBytes         []byte
	reviewKeys        []planReviewKeyEvent
	terminalErr       error
	readWake          chan struct{}
	keyWake           chan struct{}
	capacityWake      chan struct{}
	decoderState      planReviewDecoderState
	decoderGeneration uint64
	modeGeneration    uint64
	escapeTimer       *time.Timer
	dropLeadingLF     bool
	noteTransition    bool
	noteInput         bool
}

func newPlanReviewInputRouter(source planReviewInputSource) (*planReviewInputRouter, error) {
	if source == nil {
		return nil, fmt.Errorf("plan review input source is required")
	}
	return &planReviewInputRouter{
		source:       source,
		readWake:     make(chan struct{}),
		keyWake:      make(chan struct{}),
		capacityWake: make(chan struct{}),
	}, nil
}

func (router *planReviewInputRouter) Read(destination []byte) (int, error) {
	count, _, err := router.ReadWithAuthority(destination)
	return count, err
}

func (router *planReviewInputRouter) ReadWithAuthority(
	destination []byte,
) (int, terminalInputAuthority, error) {
	if router == nil || router.source == nil {
		return 0, terminalInputAuthority{}, fmt.Errorf("plan review input router is unavailable")
	}
	if len(destination) == 0 {
		return 0, terminalInputAuthority{}, nil
	}
	router.start()
	for {
		router.mu.Lock()
		if len(router.lineBytes) > 0 {
			authority := router.lineAuthority
			count := copy(destination, router.lineBytes)
			router.lineBytes = router.lineBytes[count:]
			if len(router.lineBytes) == 0 {
				router.lineAuthority = terminalInputAuthority{}
			}
			router.signalCapacityLocked()
			router.mu.Unlock()
			return count, authority, nil
		}
		if router.terminalErr != nil {
			err := router.terminalErr
			router.mu.Unlock()
			return 0, terminalInputAuthority{}, err
		}
		wake := router.readWake
		router.mu.Unlock()
		<-wake
	}
}

func (router *planReviewInputRouter) EnableReview() {
	if router == nil || router.source == nil {
		panic("plan review input router is unavailable")
	}
	router.source.LockInput()
	defer router.source.UnlockInput()
	router.mu.Lock()
	if router.review.Load() {
		router.mu.Unlock()
		router.start()
		return
	}
	if router.noteInput {
		panic("plan review cannot begin while note input is active")
	}
	router.resetDecoderLocked()
	router.reviewKeys = nil
	router.noteBytes = nil
	router.noteTransition = false
	router.noteInput = false
	router.modeGeneration++
	router.review.Store(true)
	router.signalReadLocked()
	router.signalKeyLocked()
	router.signalCapacityLocked()
	router.mu.Unlock()
	router.start()
}

func (router *planReviewInputRouter) DisableReview() {
	if router == nil || router.source == nil {
		panic("plan review input router is unavailable")
	}
	router.source.LockInput()
	defer router.source.UnlockInput()
	router.mu.Lock()
	router.disableReviewLocked(true)
	router.mu.Unlock()
}

// BeginNoteInput completes the transition started by the review Note key. The
// source bytes typed after that key remain private to the router until the
// session has accepted the key and entered note-editing state.
func (router *planReviewInputRouter) BeginNoteInput() (terminalInputAuthority, error) {
	if router == nil || router.source == nil {
		return terminalInputAuthority{}, fmt.Errorf("plan review input router is unavailable")
	}
	router.source.LockInput()
	defer router.source.UnlockInput()
	router.mu.Lock()
	defer router.mu.Unlock()
	if !router.review.Load() || !router.noteTransition {
		return terminalInputAuthority{}, fmt.Errorf("plan review note-input transition is not pending")
	}
	noteBytes := append([]byte(nil), router.noteBytes...)
	router.resetDecoderLocked()
	router.reviewKeys = nil
	router.noteBytes = nil
	router.noteTransition = false
	router.noteInput = true
	router.modeGeneration++
	router.review.Store(false)
	authority := router.currentLineAuthorityLocked()
	router.appendLineBytesLocked(noteBytes, authority)
	router.signalReadLocked()
	router.signalKeyLocked()
	router.signalCapacityLocked()
	return authority, nil
}

func (router *planReviewInputRouter) EndNoteInput(
	authority terminalInputAuthority,
) error {
	if router == nil || router.source == nil {
		return fmt.Errorf("plan review input router is unavailable")
	}
	router.source.LockInput()
	defer router.source.UnlockInput()
	router.mu.Lock()
	defer router.mu.Unlock()
	if !router.noteInput || router.currentLineAuthorityLocked() != authority ||
		authority.Mode != terminalInputPlanNote {
		return fmt.Errorf("plan review note-input authority changed")
	}
	router.noteInput = false
	router.modeGeneration++
	router.signalReadLocked()
	router.signalCapacityLocked()
	return nil
}

func (router *planReviewInputRouter) CurrentInputAuthority() terminalInputAuthority {
	if router == nil {
		return terminalInputAuthority{}
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.currentLineAuthorityLocked()
}

func (router *planReviewInputRouter) ReviewEnabled() bool {
	return router != nil && router.review.Load()
}

func (router *planReviewInputRouter) NextReviewKey(
	ctx context.Context,
) (planReviewKey, error) {
	if router == nil || router.source == nil {
		return 0, fmt.Errorf("plan review input router is unavailable")
	}
	if ctx == nil {
		return 0, fmt.Errorf("plan review key context is required")
	}
	router.mu.Lock()
	generation := router.modeGeneration
	router.mu.Unlock()
	router.start()
	for {
		router.mu.Lock()
		if len(router.reviewKeys) > 0 {
			event := router.reviewKeys[0]
			eofAfterDisable := event.Key == planReviewKeyEOF &&
				event.Generation < ^uint64(0) && event.Generation+1 == generation &&
				!router.review.Load() && router.modeGeneration == generation
			if event.Generation != generation && !eofAfterDisable {
				router.mu.Unlock()
				return 0, errPlanReviewInputInactive
			}
			router.reviewKeys = router.reviewKeys[1:]
			router.signalCapacityLocked()
			router.mu.Unlock()
			return event.Key, nil
		}
		if !router.review.Load() || router.modeGeneration != generation {
			router.mu.Unlock()
			return 0, errPlanReviewInputInactive
		}
		if router.terminalErr != nil {
			err := router.terminalErr
			router.mu.Unlock()
			return 0, err
		}
		wake := router.keyWake
		router.mu.Unlock()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-wake:
		}
	}
}

func (router *planReviewInputRouter) start() {
	router.started.Do(func() {
		go router.readSource()
	})
}

func (router *planReviewInputRouter) readSource() {
	buffer := make([]byte, 64)
	emptyReads := 0
	for {
		if !router.waitForInputCapacity() {
			return
		}
		router.source.LockInput()
		router.mu.Lock()
		if router.terminalErr != nil {
			router.mu.Unlock()
			router.source.UnlockInput()
			return
		}
		if !router.inputCapacityAvailableLocked() {
			router.mu.Unlock()
			router.source.UnlockInput()
			continue
		}
		router.mu.Unlock()

		count, err := router.source.ReadInputLocked(buffer)
		if errors.Is(err, errPlanReviewInputSourceIdle) {
			router.source.UnlockInput()
			continue
		}
		router.mu.Lock()
		if count < 0 || count > len(buffer) {
			router.finishSourceLocked(fmt.Errorf(
				"plan review input source returned invalid byte count %d",
				count,
			))
			router.mu.Unlock()
			router.source.UnlockInput()
			return
		}
		if count == 0 && err == nil {
			emptyReads++
			if emptyReads >= 100 {
				router.finishSourceLocked(io.ErrNoProgress)
				router.mu.Unlock()
				router.source.UnlockInput()
				return
			}
			router.mu.Unlock()
			router.source.UnlockInput()
			continue
		}
		emptyReads = 0

		if count > 0 {
			router.routeBytesLocked(buffer[:count])
		}
		if err != nil {
			router.finishDecoderLocked()
			router.terminalErr = err
			router.signalReadLocked()
			router.signalKeyLocked()
			router.mu.Unlock()
			router.source.UnlockInput()
			return
		}
		router.mu.Unlock()
		router.source.UnlockInput()
	}
}

func (router *planReviewInputRouter) waitForInputCapacity() bool {
	for {
		router.mu.Lock()
		if router.terminalErr != nil {
			router.mu.Unlock()
			return false
		}
		if router.inputCapacityAvailableLocked() {
			router.mu.Unlock()
			return true
		}
		wake := router.capacityWake
		router.mu.Unlock()
		<-wake
	}
}

func (router *planReviewInputRouter) inputCapacityAvailableLocked() bool {
	switch {
	case router.review.Load() && router.noteTransition:
		return len(router.reviewKeys) == 0 && len(router.noteBytes) == 0
	case router.review.Load():
		return len(router.reviewKeys) == 0
	default:
		return len(router.lineBytes) == 0
	}
}

func (router *planReviewInputRouter) finishSourceLocked(err error) {
	router.finishDecoderLocked()
	router.terminalErr = err
	router.signalReadLocked()
	router.signalKeyLocked()
	router.signalCapacityLocked()
}

func (router *planReviewInputRouter) routeBytesLocked(values []byte) {
	for index := 0; index < len(values); index++ {
		value := values[index]
		if router.dropLeadingLF {
			router.dropLeadingLF = false
			if value == '\n' {
				continue
			}
		}
		if router.noteTransition {
			router.noteBytes = append(router.noteBytes, values[index:]...)
			return
		}
		if !router.review.Load() && router.decoderState != planReviewDecoderIdle {
			router.consumeDisabledSequenceByteLocked(value)
			continue
		}
		if !router.review.Load() {
			router.appendLineBytesLocked(values[index:], router.currentLineAuthorityLocked())
			router.signalReadLocked()
			return
		}
		router.consumeReviewByteLocked(value)
	}
}

func (router *planReviewInputRouter) disableReviewLocked(clearKeys bool) {
	if router.decoderState == planReviewDecoderIdle {
		router.resetDecoderLocked()
	}
	if clearKeys {
		router.reviewKeys = nil
	}
	router.noteBytes = nil
	router.noteTransition = false
	router.noteInput = false
	router.modeGeneration++
	router.review.Store(false)
	router.signalReadLocked()
	router.signalKeyLocked()
	router.signalCapacityLocked()
}

func (router *planReviewInputRouter) currentLineAuthorityLocked() terminalInputAuthority {
	mode := terminalInputOrdinary
	if router.review.Load() {
		mode = terminalInputPlanReview
	} else if router.noteInput {
		mode = terminalInputPlanNote
	}
	return terminalInputAuthority{Generation: router.modeGeneration, Mode: mode}
}

func (router *planReviewInputRouter) appendLineBytesLocked(
	values []byte,
	authority terminalInputAuthority,
) {
	if len(values) == 0 {
		return
	}
	if len(router.lineBytes) == 0 {
		router.lineAuthority = authority
	} else if router.lineAuthority != authority {
		router.lineAuthority = terminalInputAuthority{
			Generation: authority.Generation,
			Mode:       terminalInputMixed,
		}
	}
	router.lineBytes = append(router.lineBytes, values...)
}

func (router *planReviewInputRouter) signalReadLocked() {
	close(router.readWake)
	router.readWake = make(chan struct{})
}

func (router *planReviewInputRouter) signalKeyLocked() {
	close(router.keyWake)
	router.keyWake = make(chan struct{})
}

func (router *planReviewInputRouter) signalCapacityLocked() {
	close(router.capacityWake)
	router.capacityWake = make(chan struct{})
}
