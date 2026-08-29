package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func runList(c *client.Client, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	status := fs.String("status", "", "filter by status")
	limit := fs.Int("limit", 20, "max jobs")
	offset := fs.Int("offset", 0, "offset")
	_ = fs.Parse(args)

	jobs, err := c.List(context.Background(), *status, *limit, *offset)
	if err != nil {
		die(err.Error())
	}

	if len(jobs) == 0 {
		fmt.Println("no jobs")
		return
	}

	for _, job := range jobs {
		fmt.Printf("#%d [%s] pipeline=%s created=%s\n", job.ID, job.Status, job.Pipeline, job.CreatedAt.Format(time.RFC3339))
	}
}

func runShow(c *client.Client, args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	history := fs.String("history", "", "show one history stream: generations|steps|artifacts|evidence|llm_calls")
	historyLimit := fs.Int("history-limit", 50, "history page size")
	historyCursor := fs.String("history-cursor", "", "opaque history cursor")
	_ = fs.Parse(args)
	historyPagingRequested := false
	fs.Visit(func(item *flag.Flag) {
		if item.Name == "history-limit" || item.Name == "history-cursor" {
			historyPagingRequested = true
		}
	})

	if len(fs.Args()) < 1 {
		die("show requires a job id")
	}

	id, err := strconv.ParseInt(fs.Args()[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	var payload []byte
	if *history != "" {
		page, err := c.JobHistory(context.Background(), id, queue.JobHistoryRequest{
			Stream: queue.JobHistoryStream(*history), Limit: *historyLimit, Cursor: *historyCursor,
		})
		if err != nil {
			die(err.Error())
		}
		payload, err = json.MarshalIndent(page, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
		return
	}
	if historyPagingRequested {
		die("history cursor and limit require --history")
	}

	details, err := c.Show(context.Background(), id)
	if err != nil {
		die(err.Error())
	}
	payload, err = json.MarshalIndent(details, "", "  ")
	if err != nil {
		die(err.Error())
	}
	fmt.Println(string(payload))
}

func runWatch(c *client.Client, args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	interval := fs.Duration("interval", 2*time.Second, "poll interval")
	progress := fs.Bool("progress", true, "print live stage/event updates")
	verbose := fs.Bool("verbose", false, "print full debug trace (including LLM prompts and full context dumps)")
	maxChars := fs.Int("max-chars", 1200, "max characters shown per output/context entry (0 disables truncation)")
	_ = fs.Parse(args)

	if len(fs.Args()) < 1 {
		die("watch requires a job id")
	}
	id, err := strconv.ParseInt(fs.Args()[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	lastStatus := ""
	lastStepStatus := map[int64]string{}
	lastStepDetails := map[int64]string{}
	seenContextIDs := map[int64]struct{}{}
	for {
		details, err := c.Show(context.Background(), id)
		if err != nil {
			die(err.Error())
		}

		status := details.Job.Status
		if status != lastStatus {
			fmt.Printf("job %d status=%s\n", id, status)
			lastStatus = status
		}

		printed := false
		if *progress || *verbose {
			printed = printStepStatusUpdates(details.Steps, lastStepStatus) || printed
		}
		if *verbose {
			printed = printStepDetailUpdates(details.Steps, lastStepDetails, *maxChars) || printed
		}
		if *progress || *verbose {
			printed = printContextUpdates(details.Contexts, seenContextIDs, *progress, *verbose, *maxChars) || printed
		}
		if printed {
			fmt.Println("---")
		}

		if status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusCanceled {
			if details.Job.Result != "" {
				fmt.Printf("result:\n%s\n", details.Job.Result)
			}
			if details.Job.Error != "" {
				fmt.Printf("error: %s\n", details.Job.Error)
			}
			return
		}
		if status == model.JobStatusWaiting {
			question := latestContextValue(details.Contexts, "input_question")
			if strings.TrimSpace(question) != "" {
				fmt.Printf("input requested: %s\n", question)
			} else {
				fmt.Println("input requested: core needs additional information before continuing")
			}
			fmt.Printf("provide feedback with: omni feedback %d \"...\"\n", id)
			fmt.Printf("inject extra context with: omni interrupt %d \"...\"\n", id)
			fmt.Printf("start a new generation on the same job with: omni replan %d \"...\"\n", id)
			fmt.Printf("cancel immediately with: omni cancel %d \"...\"\n", id)
			return
		}

		time.Sleep(*interval)
	}
}
