package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
)

var lifecycleOperationIDPattern = regexp.MustCompile(`^lifecycle_operation_[a-z0-9_]{1,128}$`)

type LifecycleOperationID string

func NewLifecycleOperationID() (LifecycleOperationID, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("generate lifecycle operation identity: %w", err)
	}
	return LifecycleOperationID("lifecycle_operation_" + hex.EncodeToString(nonce[:])), nil
}

func ParseLifecycleOperationID(value string) (LifecycleOperationID, error) {
	if !lifecycleOperationIDPattern.MatchString(value) {
		return "", fmt.Errorf("lifecycle operation ID requires lifecycle_operation_ plus 1..128 lowercase letters, digits, or underscores")
	}
	return LifecycleOperationID(value), nil
}
