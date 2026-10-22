package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func orderingTasks() []LedgerTask {
	return []LedgerTask{
		{LedgerItem: LedgerItem{ID: "a", Rank: 10, Disposition: model.LedgerDispositionActive, Rationale: "private rationale"}, Goal: "Create the interface.", WriteFiles: []string{"interface.go"}},
		{LedgerItem: LedgerItem{ID: "b", Rank: 30, Disposition: model.LedgerDispositionActive}, Goal: "Implement the interface.", ReadFiles: []string{"interface.go"}},
		{LedgerItem: LedgerItem{ID: "c", Rank: 20, Disposition: model.LedgerDispositionActive}, Goal: "Verify behavior.", DeleteFiles: []string{"obsolete.go"}},
	}
}

func TestTaskOrderingPermutations(t *testing.T) {
	for _, sequence := range [][]string{{"a", "b", "c"}, {"c", "a", "b"}, {"b", "c", "a"}, {"c", "b", "a"}, {"a", "c", "b"}, {"b", "a", "c"}} {
		t.Run(strings.Join(sequence, ""), func(t *testing.T) {
			tasks := orderingTasks()
			tasks[0].Goal = strings.Repeat("Required instruction. ", 10000)
			original, _ := json.Marshal(tasks)
			var before []LedgerTask
			json.Unmarshal(original, &before)
			calls := 0
			err := OrderUnorderedTasks(context.Background(), tasks, func(_ context.Context, prompt string) (string, error) {
				calls++
				if strings.Contains(prompt, "private rationale") || strings.Contains(prompt, "write_files") || strings.Contains(prompt, "disposition") {
					t.Fatal("unnecessary state in prompt")
				}
				for _, task := range tasks {
					if !strings.Contains(prompt, task.Goal) {
						t.Fatal("instructions missing")
					}
				}
				raw, _ := json.Marshal(sequence)
				return string(raw), nil
			})
			if err != nil || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			for i, id := range sequence {
				for j := range tasks {
					if string(tasks[j].ID) == id && tasks[j].Rank != int64(i+1) {
						t.Fatal("wrong rank")
					}
				}
			}
			for i := range tasks {
				tasks[i].Rank = before[i].Rank
			}
			if !reflect.DeepEqual(tasks, before) {
				t.Fatal("task content changed")
			}
		})
	}
}

func TestTaskOrderingRejectsWithoutMutation(t *testing.T) {
	for _, raw := range []string{`["b","a"]`, `["b","b","a"]`, `["b","a","foreign"]`, `["b","a","c","d"]`, `null`, `{}`, `[1,2,3]`, `["b",null,"a"]`, `["b","a","c"] true`, `["b",`, "```json\n[\"b\",\"a\",\"c\"]\n```", strings.Repeat(" ", 32<<10+1)} {
		t.Run(raw[:min(len(raw), 40)], func(t *testing.T) {
			tasks := orderingTasks()
			before, _ := json.Marshal(tasks)
			err := OrderUnorderedTasks(context.Background(), tasks, func(context.Context, string) (string, error) { return raw, nil })
			after, _ := json.Marshal(tasks)
			if err == nil || string(before) != string(after) {
				t.Fatalf("err=%v mutation=%v", err, string(before) != string(after))
			}
		})
	}
	tasks := orderingTasks()
	before, _ := json.Marshal(tasks)
	failure := errors.New("transport unavailable")
	err := OrderUnorderedTasks(context.Background(), tasks, func(context.Context, string) (string, error) { return `["c","b","a"]`, failure })
	after, _ := json.Marshal(tasks)
	if !errors.Is(err, failure) || string(before) != string(after) {
		t.Fatal("transport failure changed tasks or lost error")
	}
}

func TestTaskOrderingNoInferenceForZeroOrOne(t *testing.T) {
	for _, tasks := range [][]LedgerTask{nil, orderingTasks()[:1]} {
		before, _ := json.Marshal(tasks)
		if err := OrderUnorderedTasks(context.Background(), tasks, func(context.Context, string) (string, error) { t.Fatal("unexpected inference"); return "", nil }); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(tasks)
		if string(before) != string(after) {
			t.Fatal("mutation")
		}
	}
}

func TestTaskOrderingInvalidIdentitiesAndCancellation(t *testing.T) {
	for _, change := range []func([]LedgerTask) []LedgerTask{
		func(tasks []LedgerTask) []LedgerTask { tasks[1].ID = tasks[0].ID; return tasks },
		func(tasks []LedgerTask) []LedgerTask { tasks[0].ID = ""; return tasks },
		func(tasks []LedgerTask) []LedgerTask { return make([]LedgerTask, 129) },
	} {
		tasks := change(orderingTasks())
		before, _ := json.Marshal(tasks)
		err := OrderUnorderedTasks(context.Background(), tasks, func(context.Context, string) (string, error) {
			t.Fatal("invalid input reached transport")
			return "", nil
		})
		after, _ := json.Marshal(tasks)
		if err == nil || string(before) != string(after) {
			t.Fatal("invalid input mutated tasks or succeeded")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	tasks := orderingTasks()
	before, _ := json.Marshal(tasks)
	err := OrderUnorderedTasks(ctx, tasks, func(context.Context, string) (string, error) { cancel(); return `["c","b","a"]`, nil })
	after, _ := json.Marshal(tasks)
	if !errors.Is(err, context.Canceled) || string(before) != string(after) {
		t.Fatal("cancellation applied order")
	}
}
