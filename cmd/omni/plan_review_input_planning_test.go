package main

import (
	"strings"
	"testing"
)

func TestPlanReviewInputStartsConversationWithoutReviewScreen(t *testing.T) {
	router := mustPlanReviewInputRouter(t, strings.NewReader("continue planning\r"))
	previous := router.CurrentInputAuthority()
	authority, err := router.BeginNoteInput()
	if err != nil {
		t.Fatal(err)
	}
	if authority.Mode != terminalInputPlanNote || authority.Generation <= previous.Generation || router.ReviewEnabled() {
		t.Fatalf("planning conversation authority = %#v, previous = %#v", authority, previous)
	}
	if _, err := router.BeginNoteInput(); err == nil {
		t.Fatal("a second planning editor replaced the active input authority")
	}
	buffer := make([]byte, 64)
	count, observed, err := router.ReadWithAuthority(buffer)
	if err != nil || observed != authority || string(buffer[:count]) != "continue planning\r" {
		t.Fatalf("planning input = %q, %#v, %v", buffer[:count], observed, err)
	}
	if err := router.EndNoteInput(previous); err == nil {
		t.Fatal("ordinary input authority ended the planning editor")
	}
	if err := router.EndNoteInput(authority); err != nil {
		t.Fatal(err)
	}
	if current := router.CurrentInputAuthority(); current.Mode != terminalInputOrdinary || current.Generation <= authority.Generation {
		t.Fatalf("planning editor retained input authority: %#v", current)
	}
}

func TestPlanReviewConversationDoesNotAdoptBufferedOrdinaryInput(t *testing.T) {
	router := mustPlanReviewInputRouter(t, strings.NewReader(""))
	previous := router.CurrentInputAuthority()
	router.mu.Lock()
	router.routeBytesLocked([]byte("earlier input\r"))
	router.mu.Unlock()
	authority, err := router.BeginNoteInput()
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 64)
	count, observed, err := router.ReadWithAuthority(buffer)
	if err != nil || observed != previous || observed == authority || string(buffer[:count]) != "earlier input\r" {
		t.Fatalf("buffered ordinary input acquired planning authority: %q, %#v, %v", buffer[:count], observed, err)
	}
	if err := router.EndNoteInput(authority); err != nil {
		t.Fatal(err)
	}
}

func TestPlanReviewConversationCannotBypassPendingReviewKey(t *testing.T) {
	router := mustPlanReviewInputRouter(t, strings.NewReader(""))
	router.EnableReview()
	if _, err := router.BeginNoteInput(); err == nil {
		t.Fatal("review input entered a conversation without the Note key")
	}
}
