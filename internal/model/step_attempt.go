package model

import "time"

type StepAttemptAuthority struct {
	JobID      int64  `json:"job_id"`
	Generation int64  `json:"generation"`
	StepID     int64  `json:"step_id"`
	Attempt    int64  `json:"attempt"`
	WorkerID   string `json:"worker_id"`
}

type StepAttemptStatus string

const (
	StepAttemptActive     StepAttemptStatus = "active"
	StepAttemptCompleted  StepAttemptStatus = "completed"
	StepAttemptFailed     StepAttemptStatus = "failed"
	StepAttemptWaiting    StepAttemptStatus = "waiting_input"
	StepAttemptCanceled   StepAttemptStatus = "canceled"
	StepAttemptSuperseded StepAttemptStatus = "superseded"
	StepAttemptExpired    StepAttemptStatus = "expired"
)

type StepAttempt struct {
	StepAttemptAuthority
	Status     StepAttemptStatus `json:"status"`
	ClaimedAt  time.Time         `json:"claimed_at"`
	RenewedAt  time.Time         `json:"renewed_at"`
	ExpiresAt  time.Time         `json:"expires_at"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
}
