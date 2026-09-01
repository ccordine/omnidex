package main

import (
	"fmt"
	"time"
)

const planReviewEscapeTimeout = 40 * time.Millisecond

type planReviewKey uint8

const (
	planReviewKeyUp planReviewKey = iota + 1
	planReviewKeyDown
	planReviewKeyToggle
	planReviewKeyEnter
	planReviewKeyEscape
	planReviewKeyNote
	planReviewKeyEOF
)

type planReviewDecoderState uint8

const (
	planReviewDecoderIdle planReviewDecoderState = iota
	planReviewDecoderEscape
	planReviewDecoderCSI
	planReviewDecoderSS3
)

func (router *planReviewInputRouter) consumeReviewByteLocked(value byte) {
	switch router.decoderState {
	case planReviewDecoderIdle:
		switch value {
		case '\x1b':
			router.decoderState = planReviewDecoderEscape
			router.armDecoderTimerLocked()
		case ' ':
			router.appendReviewKeyLocked(planReviewKeyToggle)
		case '\r':
			router.dropLeadingLF = true
			router.appendReviewKeyLocked(planReviewKeyEnter)
		case '\n':
			router.appendReviewKeyLocked(planReviewKeyEnter)
		case 'n', 'N':
			router.appendReviewKeyLocked(planReviewKeyNote)
			router.noteTransition = true
		case '\x04':
			router.appendReviewKeyLocked(planReviewKeyEOF)
			router.disableReviewLocked(false)
		}
	case planReviewDecoderEscape:
		router.stopDecoderTimerLocked()
		switch value {
		case '[':
			router.decoderState = planReviewDecoderCSI
			router.armDecoderTimerLocked()
		case 'O':
			router.decoderState = planReviewDecoderSS3
			router.armDecoderTimerLocked()
		default:
			router.decoderState = planReviewDecoderIdle
		}
	case planReviewDecoderCSI, planReviewDecoderSS3:
		router.stopDecoderTimerLocked()
		router.decoderState = planReviewDecoderIdle
		switch value {
		case 'A':
			router.appendReviewKeyLocked(planReviewKeyUp)
		case 'B':
			router.appendReviewKeyLocked(planReviewKeyDown)
		}
	default:
		panic(fmt.Sprintf("unsupported plan review decoder state %d", router.decoderState))
	}
}

func (router *planReviewInputRouter) consumeDisabledSequenceByteLocked(value byte) {
	switch router.decoderState {
	case planReviewDecoderEscape:
		router.stopDecoderTimerLocked()
		if value == '[' {
			router.decoderState = planReviewDecoderCSI
			router.armDecoderTimerLocked()
			return
		}
		if value == 'O' {
			router.decoderState = planReviewDecoderSS3
			router.armDecoderTimerLocked()
			return
		}
		router.decoderState = planReviewDecoderIdle
	case planReviewDecoderCSI, planReviewDecoderSS3:
		router.resetDecoderLocked()
	default:
		panic(fmt.Sprintf("unsupported disabled review decoder state %d", router.decoderState))
	}
}

func (router *planReviewInputRouter) appendReviewKeyLocked(key planReviewKey) {
	router.reviewKeys = append(router.reviewKeys, planReviewKeyEvent{
		Key: key, Generation: router.modeGeneration,
	})
	router.signalKeyLocked()
}

func (router *planReviewInputRouter) finishDecoderLocked() {
	state := router.decoderState
	router.resetDecoderLocked()
	if state == planReviewDecoderEscape && router.review.Load() {
		router.appendReviewKeyLocked(planReviewKeyEscape)
	}
}

func (router *planReviewInputRouter) resetDecoderLocked() {
	router.stopDecoderTimerLocked()
	router.decoderState = planReviewDecoderIdle
}

func (router *planReviewInputRouter) armDecoderTimerLocked() {
	router.decoderGeneration++
	generation := router.decoderGeneration
	router.escapeTimer = time.AfterFunc(planReviewEscapeTimeout, func() {
		router.expireDecoder(generation)
	})
}

func (router *planReviewInputRouter) stopDecoderTimerLocked() {
	router.decoderGeneration++
	if router.escapeTimer != nil {
		router.escapeTimer.Stop()
		router.escapeTimer = nil
	}
}

func (router *planReviewInputRouter) expireDecoder(generation uint64) {
	router.mu.Lock()
	defer router.mu.Unlock()
	if generation != router.decoderGeneration ||
		router.decoderState == planReviewDecoderIdle {
		return
	}
	state := router.decoderState
	router.escapeTimer = nil
	router.decoderState = planReviewDecoderIdle
	if state == planReviewDecoderEscape && router.review.Load() {
		router.appendReviewKeyLocked(planReviewKeyEscape)
	}
}
