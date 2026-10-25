package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const progressReport = "The first file is written and verified. The second file and its verification remain unfinished. No owner decision or unavailable capability prevents continuing."

type progressEvidence struct {
	durableTaskEvidence
	after func(Entry) error
}

func (s *progressEvidence) Append(ctx context.Context, e Entry) (int64, error) {
	id, err := s.durableTaskEvidence.Append(ctx, e)
	if err == nil && s.after != nil {
		err = s.after(e)
	}
	return id, err
}

type progressEnvironment struct{ blocked bool }

func (e *progressEnvironment) Attest(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.blocked {
		return errors.New("fixture execution authority withdrawn")
	}
	return nil
}

type progressProvider struct {
	t                   *testing.T
	roles               []Role
	executions, reviews int
	finalKind           ExecutionResponseKind
	rejectFirstReview   bool
	retainedFinalCallID int64
	ownerClarification  string
}

func (p *progressProvider) Run(_ context.Context, a Assignment, emit func(NativeEvent) error) (Result, error) {
	p.roles = append(p.roles, a.Role)
	if a.Task == nil || a.Task.ID != "selected-task" || a.Route.Model != "fixture" || a.Route.Effort != "low" {
		p.t.Fatal("continuation changed task or captured route")
	}
	thread, turn := "task-thread", fmt.Sprintf("turn-%d", p.executions)
	var content json.RawMessage
	switch a.Role {
	case Execute:
		p.executions++
		turn = fmt.Sprintf("turn-%d", p.executions)
		file := filepath.Join(a.Root, "first.txt")
		report := "Both files and their required checks are complete."
		switch p.executions {
		case 1:
			if a.Session != nil || a.ExecutionReport != nil {
				p.t.Fatal("initial execution was not fresh")
			}
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				p.t.Fatal("first effect replayed")
			}
			report = progressReport
		case 2:
			if a.Session == nil || a.Session.ThreadID != thread || a.Session.TurnID != "turn-1" || a.Session.State != "idle" ||
				a.ExecutionReport == nil || a.ExecutionReport.Response != progressReport || a.ExecutionReport.CallID < 1 || a.ExecutionReport.Round != 1 || a.Reply != "" {
				p.t.Fatalf("partial execution lost its session, report or authority: %+v", a)
			}
			if bytes, err := os.ReadFile(file); err != nil || string(bytes) != "effect-1" {
				p.t.Fatal("retained first effect changed", err)
			}
			file = filepath.Join(a.Root, "second.txt")
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				p.t.Fatal("second effect replayed")
			}
			if p.finalKind != ExecutionDone {
				report = "An actual owner answer, authorization or unavailable runtime now prevents completion."
			}
		case 3:
			if !p.rejectFirstReview || a.Session != nil || a.ExecutionReport != nil || len(a.Findings) != 1 {
				p.t.Fatal("review correction did not start fresh with its finding")
			}
			thread = "correction-thread"
			file = filepath.Join(a.Root, "second.txt")
		default:
			p.t.Fatal("unexpected execution replay")
		}
		if err := os.WriteFile(file, []byte(fmt.Sprintf("effect-%d", p.executions)), 0600); err != nil {
			p.t.Fatal(err)
		}
		content, _ = json.Marshal(report)
	case ExecutionResponse:
		if p.ownerClarification != "" && (len(a.Clarifications) != 1 || a.Clarifications[0].Answer != p.ownerClarification) {
			p.t.Fatal("execution routing lost the current task's owner clarification or leaked another task's answer")
		}
		kind := p.finalKind
		if p.executions == 1 {
			kind = ExecutionContinue
			if a.Response != progressReport {
				p.t.Fatal("partial response was rewritten")
			}
		}
		content, _ = json.Marshal(map[string]ExecutionResponseKind{"kind": kind})
	case Review:
		p.reviews++
		if p.executions < 2 || a.Session != nil || a.ExecutionReport == nil || a.ExecutionReport.Response == progressReport || a.ExecutionReport.CallID < 1 {
			p.t.Fatal("review ran on partial progress or lost final evidence")
		}
		p.retainedFinalCallID = a.ExecutionReport.CallID
		content = json.RawMessage(`"Independent verification complete."`)
		if p.rejectFirstReview && p.reviews == 1 {
			content = json.RawMessage(`"Inspected both files. Correct the second output in second.txt."`)
		}
	case ReviewResponse:
		if p.rejectFirstReview && p.reviews == 1 {
			content = json.RawMessage(`{"kind":"done","findings":[{"task_id":"selected-task","file":"second.txt","observation":"Correct the second output."}],"verification":"Inspected both files."}`)
		} else {
			content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Inspected both files."}`)
		}
	default:
		p.t.Fatalf("unexpected stage %s", a.Role)
	}
	if err := emit(NativeEvent{Frame: json.RawMessage(`{"fixture":"partial-progress"}`)}); err != nil {
		return Result{}, err
	}
	return Result{Provider: a.Route.Provider, Model: a.Route.Model, Completed: true, ThreadID: thread, TurnID: turn, Content: content}, nil
}

func progressFixture(t *testing.T) (Controller, Task, *progressProvider, *progressEvidence, *progressEnvironment) {
	t.Helper()
	task := Task{ID: "selected-task", Goal: "Write first.txt and second.txt and verify both.", WriteFiles: []string{"first.txt", "second.txt"}}
	p := &progressProvider{t: t, finalKind: ExecutionDone}
	s := &progressEvidence{}
	env := &progressEnvironment{}
	route := Route{Provider: "fixture", Model: "fixture", Effort: "low", ServiceTier: "default"}
	c := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Evidence: s, Environment: env,
		Providers: map[string]Provider{"fixture": p}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
	return c, task, p, s, env
}

func TestPartialExecutionContinuesBeforeReviewAndAcrossRestart(t *testing.T) {
	for _, boundary := range []string{"uninterrupted", "native-result", "continuation-decision", "next-dispatch"} {
		t.Run(boundary, func(t *testing.T) {
			c, task, p, s, _ := progressFixture(t)
			crash := errors.New("fixture process stopped after durable write")
			s.after = func(e Entry) error {
				stop := boundary == "native-result" && e.Phase == "call_completed" && e.Role == Execute ||
					boundary == "continuation-decision" && e.Phase == "execution_response" && e.ResponseKind == ExecutionContinue ||
					boundary == "next-dispatch" && e.Phase == "task_executing" && e.Round == 2
				if stop {
					s.after = nil
					return crash
				}
				return nil
			}
			err := c.RunTask(t.Context(), task)
			if boundary != "uninterrupted" {
				if !errors.Is(err, crash) || p.executions != 1 || p.reviews != 0 {
					t.Fatalf("wrong checkpoint: %v executions=%d reviews=%d", err, p.executions, p.reviews)
				}
				restarted := c
				err = restarted.RunTask(t.Context(), task)
			}
			if err != nil {
				t.Fatal(err)
			}
			want := []Role{Execute, ExecutionResponse, Execute, ExecutionResponse, Review, ReviewResponse}
			if !reflect.DeepEqual(p.roles, want) || p.retainedFinalCallID < 1 {
				t.Fatalf("unexpected stages or review evidence: %v", p.roles)
			}
			progress, accepted := 0, 0
			for _, e := range s.entries {
				if e.Phase == "execution_response" && e.ResponseKind == ExecutionContinue {
					progress++
					if !reflect.DeepEqual(e.Notes, []string{progressReport}) {
						t.Fatal("progress evidence was changed")
					}
				}
				if e.Phase == "task_accepted" {
					accepted++
				}
			}
			if progress != 1 || accepted != 1 {
				t.Fatalf("progress=%d accepted=%d", progress, accepted)
			}
			calls := len(p.roles)
			if err := c.RunTask(t.Context(), task); err != nil || len(p.roles) != calls {
				t.Fatal("accepted work replayed", err)
			}
		})
	}
}

func TestPartialExecutionPreservesCancellationAuthorityAndBlockers(t *testing.T) {
	for _, boundary := range []string{"cancel", "authority", "question", "permission", "error"} {
		t.Run(boundary, func(t *testing.T) {
			c, task, p, s, env := progressFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if boundary == "cancel" || boundary == "authority" {
				s.after = func(e Entry) error {
					if e.Phase == "execution_response" && e.ResponseKind == ExecutionContinue {
						if boundary == "cancel" {
							cancel()
						} else {
							env.blocked = true
						}
					}
					return nil
				}
			} else {
				p.finalKind = ExecutionResponseKind(boundary)
			}
			err := c.RunTask(ctx, task)
			if err == nil || p.reviews != 0 {
				t.Fatalf("blocked work reached review: %v", err)
			}
			if boundary == "cancel" && (!errors.Is(err, context.Canceled) || p.executions != 1) {
				t.Fatal("cancellation did not prevent continuation", err)
			}
			if boundary == "authority" && p.executions != 1 {
				t.Fatal("continuation escaped execution authority")
			}
			if boundary == "question" || boundary == "permission" {
				var input *ExecutionInputRequired
				if !errors.As(err, &input) || string(input.Kind) != boundary {
					t.Fatalf("owner input route lost: %v", err)
				}
			}
			if boundary == "error" {
				var incomplete *IncompleteTaskError
				if !errors.As(err, &incomplete) {
					t.Fatal("genuine blocker was swallowed", err)
				}
			}
			for _, e := range s.entries {
				if e.Phase == "task_accepted" {
					t.Fatal("blocked work was accepted")
				}
			}
		})
	}
}

func TestPartialExecutionStillUsesFreshCorrectionAfterReview(t *testing.T) {
	c, task, p, _, _ := progressFixture(t)
	p.rejectFirstReview = true
	if err := c.RunTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if p.executions != 3 || p.reviews != 2 {
		t.Fatalf("correction changed: executions=%d reviews=%d", p.executions, p.reviews)
	}
}

func TestExecutionContinuationRoutesWithOnlyItsOwnerClarifications(t *testing.T) {
	c, task, p, s, _ := progressFixture(t)
	p.ownerClarification = "Skip the optional visual check; finish the two files and their automated checks."
	s.entries = []Entry{
		{Phase: "execution_response", TaskID: task.ID, CallID: 41, Round: 1, ResponseKind: ExecutionQuestion, Notes: []string{"Is the optional visual check required?"}},
		{Phase: "execution_reply", TaskID: task.ID, CallID: 41, Notes: []string{"answer-41", p.ownerClarification}},
		{Phase: "execution_response", TaskID: "another-task", CallID: 42, Round: 1, ResponseKind: ExecutionQuestion, Notes: []string{"Different question"}},
		{Phase: "execution_reply", TaskID: "another-task", CallID: 42, Notes: []string{"answer-42", "Unrelated owner answer"}},
	}
	if err := c.RunTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
}
