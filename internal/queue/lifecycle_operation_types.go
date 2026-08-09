package queue

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const lifecycleOperationIdentitySchema = "omnidex.lifecycle-operation-identity.v1"

var (
	ErrLifecycleOperationConflict = errors.New("lifecycle operation identity conflict")
	lifecycleOperationIDPattern   = regexp.MustCompile(`^lifecycle_operation_[0-9a-f]{64}$`)
)

type LifecycleOperationID string

func NewLifecycleOperationID(parts ...string) (LifecycleOperationID, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("lifecycle operation identity requires at least one part")
	}
	for index, part := range parts {
		if part == "" || part != strings.TrimSpace(part) {
			return "", fmt.Errorf("lifecycle operation identity part %d must be one nonempty exact value", index)
		}
	}
	return LifecycleOperationID("lifecycle_operation_" + lifecycleIdentityDigest(
		append([]string{lifecycleOperationIdentitySchema}, parts...)...,
	)), nil
}

func NewRandomLifecycleOperationID() (LifecycleOperationID, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("generate lifecycle operation identity: %w", err)
	}
	return NewLifecycleOperationID("random", hex.EncodeToString(nonce[:]))
}

func ParseLifecycleOperationID(value string) (LifecycleOperationID, error) {
	id := LifecycleOperationID(value)
	if !lifecycleOperationIDPattern.MatchString(value) {
		return "", fmt.Errorf("lifecycle operation ID must match lifecycle_operation_ plus 64 lowercase hex characters")
	}
	return id, nil
}

type LifecycleOperationKind string

const (
	LifecycleCompleteStep   LifecycleOperationKind = "complete_step"
	LifecycleFailStep       LifecycleOperationKind = "fail_step"
	LifecycleSubmitFeedback LifecycleOperationKind = "submit_feedback"
	LifecycleReplanJob      LifecycleOperationKind = "replan_job"
	LifecycleScrumChannel   LifecycleOperationKind = "scrum_channel_message"
	LifecycleCancelJob      LifecycleOperationKind = "cancel_job"
)

type CompleteStepCommand struct {
	OperationID  LifecycleOperationID `json:"operation_id"`
	StepID       int64                `json:"step_id"`
	Output       string               `json:"output"`
	ContextKey   string               `json:"context_key"`
	ContextValue string               `json:"context_value"`
}

type FailStepCommand struct {
	OperationID LifecycleOperationID `json:"operation_id"`
	StepID      int64                `json:"step_id"`
	Error       string               `json:"error"`
}

type SubmitJobFeedbackCommand struct {
	OperationID LifecycleOperationID `json:"operation_id"`
	JobID       int64                `json:"job_id"`
	Feedback    string               `json:"feedback"`
}

type ReplanJobCommand struct {
	OperationID LifecycleOperationID `json:"operation_id"`
	JobID       int64                `json:"job_id"`
	Feedback    string               `json:"feedback"`
}

type CancelJobCommand struct {
	OperationID LifecycleOperationID `json:"operation_id"`
	JobID       int64                `json:"job_id"`
	Reason      string               `json:"reason"`
}

func lifecycleIdentityDigest(parts ...string) string {
	var canonical bytes.Buffer
	for _, part := range parts {
		_ = binary.Write(&canonical, binary.BigEndian, uint64(len(part)))
		_, _ = canonical.WriteString(part)
	}
	digest := sha256.Sum256(canonical.Bytes())
	return hex.EncodeToString(digest[:])
}
