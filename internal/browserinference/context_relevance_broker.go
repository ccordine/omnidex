package browserinference

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

const (
	ContextRelevanceJobSchemaV1        = "omnidex.browser-context-relevance-job.v1"
	ContextRelevanceSubmissionSchemaV1 = "omnidex.browser-context-relevance-result.v1"
	MaxContextRelevanceResultBytes     = 16 * 1024
	MaxContextRelevanceFailureBytes    = 2 * 1024
	MaxContextRelevanceSubmissionBytes = 128 * 1024
	ContextRelevanceMaxOutputTokens    = 256
)

var (
	ErrNoBrowserWorker           = errors.New("no ready browser context relevance worker")
	ErrBrowserWorkerConnected    = errors.New("a browser context relevance worker is already connected")
	ErrUnknownBrowserJob         = errors.New("browser context relevance job is not pending")
	ErrBrowserWorkerDisconnected = errors.New("browser context relevance worker disconnected")
)

type ContextRelevanceJob struct {
	Schema          string         `json:"schema"`
	JobID           string         `json:"job_id"`
	Station         station.ID     `json:"station"`
	Model           string         `json:"model"`
	Prompt          string         `json:"prompt"`
	PromptHint      string         `json:"prompt_hint"`
	ResponseSchema  map[string]any `json:"response_schema"`
	MaxOutputTokens int            `json:"max_output_tokens"`
}

type ContextRelevanceSubmission struct {
	Schema    string `json:"schema"`
	JobID     string `json:"job_id"`
	Model     string `json:"model"`
	RawResult string `json:"raw_result,omitempty"`
	Error     string `json:"error,omitempty"`
}

type contextRelevanceOutcome struct {
	decision assemblyline.ContextRelevanceDecision
	err      error
}

type contextRelevanceRequest struct {
	packet    ContextRelevanceJob
	input     assemblyline.ContextRelevanceInput
	session   *ContextRelevanceSession
	result    chan contextRelevanceOutcome
	delivered bool
}

type ContextRelevanceBroker struct {
	mu      sync.Mutex
	active  *ContextRelevanceSession
	jobs    chan *contextRelevanceRequest
	pending map[string]*contextRelevanceRequest
}

type ContextRelevanceSession struct {
	broker *ContextRelevanceBroker
}

func NewContextRelevanceBroker() *ContextRelevanceBroker {
	return &ContextRelevanceBroker{
		jobs:    make(chan *contextRelevanceRequest, 64),
		pending: make(map[string]*contextRelevanceRequest),
	}
}

func (broker *ContextRelevanceBroker) Connect() (*ContextRelevanceSession, error) {
	if broker == nil {
		return nil, fmt.Errorf("browser context relevance broker is unavailable")
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.active != nil {
		return nil, ErrBrowserWorkerConnected
	}
	session := &ContextRelevanceSession{broker: broker}
	broker.active = session
	return session, nil
}

func (broker *ContextRelevanceBroker) ExecuteContextRelevance(
	ctx context.Context,
	model string,
	input assemblyline.ContextRelevanceInput,
) (assemblyline.ContextRelevanceDecision, error) {
	if ctx == nil {
		return assemblyline.ContextRelevanceDecision{}, fmt.Errorf("browser context relevance requires a context")
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.ContextRelevanceDecision{}, err
	}
	if err := validateBrowserModel(model); err != nil {
		return assemblyline.ContextRelevanceDecision{}, err
	}
	job, err := assemblyline.NewContextRelevanceJob(input)
	if err != nil {
		return assemblyline.ContextRelevanceDecision{}, err
	}
	prompt, responseSchema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.ContextRelevanceDecision{}, err
	}
	jobID, err := newContextRelevanceJobID()
	if err != nil {
		return assemblyline.ContextRelevanceDecision{}, err
	}
	request := &contextRelevanceRequest{
		packet: ContextRelevanceJob{
			Schema: ContextRelevanceJobSchemaV1, JobID: jobID,
			Station: station.ContextRelevance, Model: model,
			Prompt: prompt, PromptHint: llm.MinimalGeneratePrompt,
			ResponseSchema: responseSchema, MaxOutputTokens: ContextRelevanceMaxOutputTokens,
		},
		input: cloneContextRelevanceInput(input), result: make(chan contextRelevanceOutcome, 1),
	}

	broker.mu.Lock()
	if broker.active == nil {
		broker.mu.Unlock()
		return assemblyline.ContextRelevanceDecision{}, ErrNoBrowserWorker
	}
	request.session = broker.active
	broker.pending[jobID] = request
	broker.mu.Unlock()
	defer broker.remove(jobID, request)

	select {
	case broker.jobs <- request:
	case <-ctx.Done():
		return assemblyline.ContextRelevanceDecision{}, ctx.Err()
	}
	select {
	case outcome := <-request.result:
		return outcome.decision, outcome.err
	case <-ctx.Done():
		return assemblyline.ContextRelevanceDecision{}, ctx.Err()
	}
}

func (session *ContextRelevanceSession) Next(ctx context.Context) (ContextRelevanceJob, error) {
	if ctx == nil || session == nil || session.broker == nil {
		return ContextRelevanceJob{}, fmt.Errorf("browser context relevance session is unavailable")
	}
	for {
		select {
		case request := <-session.broker.jobs:
			session.broker.mu.Lock()
			current, exists := session.broker.pending[request.packet.JobID]
			active := session.broker.active == session
			valid := exists && current == request && request.session == session && !request.delivered
			session.broker.mu.Unlock()
			if !active {
				return ContextRelevanceJob{}, ErrBrowserWorkerDisconnected
			}
			if valid {
				return request.packet, nil
			}
		case <-ctx.Done():
			return ContextRelevanceJob{}, ctx.Err()
		}
	}
}

func (session *ContextRelevanceSession) Submit(submission ContextRelevanceSubmission) error {
	if session == nil || session.broker == nil {
		return fmt.Errorf("browser context relevance session is unavailable")
	}
	broker := session.broker
	broker.mu.Lock()
	request, exists := broker.pending[submission.JobID]
	if !exists || request.session != session || broker.active != session || request.delivered {
		broker.mu.Unlock()
		return ErrUnknownBrowserJob
	}
	validationErr := validateContextRelevanceSubmission(submission, request.packet)
	var decision assemblyline.ContextRelevanceDecision
	if validationErr == nil && submission.Error == "" {
		decision, validationErr = assemblyline.DecodeContextRelevanceDecision(
			request.input, submission.RawResult,
		)
	}
	if validationErr == nil && submission.Error != "" {
		validationErr = fmt.Errorf("browser context relevance inference failed: %s", submission.Error)
	}
	request.delivered = true
	broker.mu.Unlock()
	request.result <- contextRelevanceOutcome{decision: decision, err: validationErr}
	return validationErr
}

func (session *ContextRelevanceSession) Close(cause error) {
	if session == nil || session.broker == nil {
		return
	}
	if cause == nil {
		cause = ErrBrowserWorkerDisconnected
	}
	broker := session.broker
	broker.mu.Lock()
	if broker.active != session {
		broker.mu.Unlock()
		return
	}
	broker.active = nil
	pending := make([]*contextRelevanceRequest, 0, len(broker.pending))
	for _, request := range broker.pending {
		if request.session == session && !request.delivered {
			request.delivered = true
			pending = append(pending, request)
		}
	}
	broker.mu.Unlock()
	for _, request := range pending {
		request.result <- contextRelevanceOutcome{
			err: fmt.Errorf("%w: %w", ErrBrowserWorkerDisconnected, cause),
		}
	}
}

func (broker *ContextRelevanceBroker) remove(
	jobID string,
	request *contextRelevanceRequest,
) {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.pending[jobID] == request {
		delete(broker.pending, jobID)
	}
}

func validateContextRelevanceSubmission(
	submission ContextRelevanceSubmission,
	packet ContextRelevanceJob,
) error {
	if submission.Schema != ContextRelevanceSubmissionSchemaV1 {
		return fmt.Errorf("browser context relevance result has invalid schema")
	}
	if submission.JobID != packet.JobID || submission.Model != packet.Model {
		return fmt.Errorf("browser context relevance result differs from its exact job authority")
	}
	if (submission.RawResult == "") == (submission.Error == "") {
		return fmt.Errorf("browser context relevance result requires exactly one raw_result or error")
	}
	if submission.RawResult != "" && (len(submission.RawResult) > MaxContextRelevanceResultBytes ||
		!utf8.ValidString(submission.RawResult) || strings.ContainsRune(submission.RawResult, '\x00')) {
		return fmt.Errorf("browser context relevance raw result exceeds its transport boundary")
	}
	if submission.Error != "" && (submission.Error != strings.TrimSpace(submission.Error) ||
		len(submission.Error) > MaxContextRelevanceFailureBytes || !utf8.ValidString(submission.Error) ||
		strings.ContainsRune(submission.Error, '\x00')) {
		return fmt.Errorf("browser context relevance failure is invalid")
	}
	return nil
}

func cloneContextRelevanceInput(
	input assemblyline.ContextRelevanceInput,
) assemblyline.ContextRelevanceInput {
	input.RetrievalConcepts = append([]string(nil), input.RetrievalConcepts...)
	input.CandidateAuthorities = append(
		[]assemblyline.ContextCandidateAuthority(nil), input.CandidateAuthorities...,
	)
	return input
}

func validateBrowserModel(model string) error {
	if model == "" || model != strings.TrimSpace(model) || len(model) > 256 ||
		!utf8.ValidString(model) || strings.ContainsRune(model, '\x00') {
		return fmt.Errorf("browser context relevance model is invalid")
	}
	return nil
}

func newContextRelevanceJobID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("create browser context relevance job ID: %w", err)
	}
	return "bcr_" + hex.EncodeToString(raw[:]), nil
}
