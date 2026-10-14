package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	MaxPlanningActivityBytes         = 48 * 1024
	MaxPlanningCandidateSubjectBytes = 1024
	MaxPlanningSemanticSubjectBytes  = 2000
)

var ErrPlanningActivityTooLarge = errors.New("planning observation exceeds the existing realtime publication boundary")

// PlanningActivity is a public observation of code-owned planning facts.
// Candidate text is a proposal until its validated disposition is observed.
// It carries no native identity, model authority or instruction payload.
type PlanningActivity struct {
	Schema        int      `json:"schema"`
	Kind          string   `json:"kind"`
	Subject       string   `json:"subject,omitempty"`
	Outcome       string   `json:"outcome,omitempty"`
	Count         int      `json:"count,omitempty"`
	WorkKind      string   `json:"workKind,omitempty"`
	Station       string   `json:"station,omitempty"`
	Model         string   `json:"model,omitempty"`
	CallID        int64    `json:"callID,omitempty"`
	Question      string   `json:"question,omitempty"`
	Why           string   `json:"why,omitempty"`
	Options       []string `json:"options,omitempty"`
	Consumer      string   `json:"consumer,omitempty"`
	Against       string   `json:"against,omitempty"`
	Result        string   `json:"result,omitempty"`
	Truncated     bool     `json:"truncated,omitempty"`
	InferenceFree bool     `json:"inferenceFree,omitempty"`
	OptionOffset  int      `json:"optionOffset,omitempty"`
	OptionTotal   int      `json:"optionTotal,omitempty"`
	ResultOffset  int      `json:"resultOffset,omitempty"`
	ResultTotal   int      `json:"resultTotal,omitempty"`
}

func (activity PlanningActivity) Validate() error {
	if activity.Schema != 1 || activity.Count < 0 {
		return fmt.Errorf("planning activity requires its supported schema and observed nonnegative count")
	}
	if activity.Kind == "semantic_started" || activity.Kind == "semantic_completed" {
		return activity.validateSemantic()
	}
	if activity.WorkKind != "" || activity.Station != "" || activity.Model != "" || activity.CallID != 0 || activity.Question != "" || activity.Why != "" || activity.Options != nil || activity.Consumer != "" || activity.Against != "" || activity.Result != "" || activity.Truncated || activity.InferenceFree || activity.OptionOffset != 0 || activity.OptionTotal != 0 || activity.ResultOffset != 0 || activity.ResultTotal != 0 {
		return fmt.Errorf("planning candidate observation carries unrelated semantic call facts")
	}
	switch activity.Kind {
	case "inventory_started", "inventory_completed", "selected":
		if activity.Subject != "" || activity.Outcome != "" || activity.Kind == "inventory_started" && activity.Count != 0 {
			return fmt.Errorf("planning inventory/selection carries unrelated candidate facts")
		}
		return nil
	case "candidate_started", "candidate_resolved":
		if activity.Subject == "" || activity.Subject != strings.TrimSpace(activity.Subject) ||
			!activityText(activity.Subject, MaxPlanningCandidateSubjectBytes) || strings.ContainsAny(activity.Subject, "\r\n") {
			return fmt.Errorf("planning candidate requires its exact bounded statement")
		}
	default:
		return fmt.Errorf("planning activity kind is unregistered")
	}
	if activity.Kind == "candidate_started" {
		if activity.Outcome != "" || activity.Count != 0 {
			return fmt.Errorf("unresolved planning candidate cannot claim an outcome")
		}
		return nil
	}
	switch activity.Outcome {
	case "partitioned":
		return nil
	case "retained", "excluded", "unrequested", "duplicate", "unresolved", "deferred":
		if activity.Count != 0 {
			return fmt.Errorf("planning disposition carries an unrelated partition count")
		}
		return nil
	default:
		return fmt.Errorf("planning disposition is unregistered")
	}
}

func (activity PlanningActivity) validateSemantic() error {
	if activity.CallID < 0 || activity.WorkKind == "" || activity.Station == "" {
		return fmt.Errorf("semantic observation requires its exact work and valid existing call identity")
	}
	for _, value := range []string{activity.WorkKind, activity.Station, activity.Question, activity.Why, activity.Consumer} {
		if value == "" || strings.TrimSpace(value) != value || !activityText(value, 4096) {
			return fmt.Errorf("semantic observation lacks its exact question, reason or consumer")
		}
	}
	if (!activity.InferenceFree && activity.Model == "") || !activityText(activity.Model, 256) || activity.Model != strings.TrimSpace(activity.Model) || activity.Outcome != "" || activity.Count != 0 {
		return fmt.Errorf("semantic observation changed its inference or planning scope")
	}
	for _, value := range []string{activity.Subject, activity.Against} {
		if !activityText(value, MaxPlanningSemanticSubjectBytes) {
			return fmt.Errorf("semantic observation carries an invalid candidate or comparison")
		}
	}
	if len(activity.Options) == 0 && !(activity.Kind == "semantic_completed" && activity.OptionTotal > 0) {
		return fmt.Errorf("semantic observation omits the offered choices or permitted output")
	}
	for _, option := range activity.Options {
		if option == "" || option != strings.TrimSpace(option) || !activityText(option, 4096+8) {
			return fmt.Errorf("semantic observation carries an invalid offered option")
		}
	}
	if activity.OptionOffset < 0 || activity.OptionTotal < 0 || (activity.OptionTotal == 0 && activity.OptionOffset != 0) || (activity.OptionTotal > 0 && (activity.OptionOffset > activity.OptionTotal || len(activity.Options) > activity.OptionTotal-activity.OptionOffset)) {
		return fmt.Errorf("semantic option page changed its original offered-choice bounds")
	}
	if activity.ResultOffset < 0 || activity.ResultTotal < 0 || (activity.ResultTotal == 0 && activity.ResultOffset != 0) || (activity.ResultTotal > 0 && (activity.ResultOffset > activity.ResultTotal || len(activity.Result) > activity.ResultTotal-activity.ResultOffset)) {
		return fmt.Errorf("semantic result page changed its original validated-result bounds")
	}
	if activity.Kind == "semantic_started" {
		if activity.Result != "" || activity.Truncated || activity.ResultOffset != 0 || activity.ResultTotal != 0 {
			return fmt.Errorf("unresolved semantic observation cannot claim a result")
		}
	} else if activity.Result == "" || !activityText(activity.Result, MaxAgentActivityTextBytes) {
		return fmt.Errorf("completed semantic observation lacks its validated result")
	}
	encoded, err := json.Marshal(activity)
	if err != nil {
		return err
	}
	if len(encoded) > MaxPlanningActivityBytes {
		return ErrPlanningActivityTooLarge
	}
	return nil
}
