package projectgit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	ChangedFileLimit = 64
	CommitLimit      = 12
)

type ChangedFile struct {
	Path           string `json:"path"`
	IndexStatus    string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
	Status         string `json:"status"`
}

type Commit struct {
	Hash         string `json:"hash"`
	Subject      string `json:"subject"`
	Author       string `json:"author"`
	RelativeDate string `json:"relative_date"`
}

type Status struct {
	Location          string        `json:"location"`
	RequestedLocation string        `json:"requested_location"`
	Source            string        `json:"source"`
	IsRepo            bool          `json:"is_repo"`
	Message           string        `json:"message"`
	Root              string        `json:"root"`
	Branch            string        `json:"branch"`
	Detached          bool          `json:"detached"`
	HeadShort         string        `json:"head_short"`
	HasUpstream       bool          `json:"has_upstream"`
	UpstreamBranch    string        `json:"upstream_branch"`
	Ahead             int           `json:"ahead"`
	Behind            int           `json:"behind"`
	RemoteURL         string        `json:"remote_url"`
	StagedCount       int           `json:"staged_count"`
	ModifiedCount     int           `json:"modified_count"`
	UntrackedCount    int           `json:"untracked_count"`
	DeletedCount      int           `json:"deleted_count"`
	ConflictedCount   int           `json:"conflicted_count"`
	ChangedFiles      []ChangedFile `json:"changed_files"`
	Clean             bool          `json:"clean"`
	StashCount        int           `json:"stash_count"`
	RecentCommits     []Commit      `json:"recent_commits"`
	LastCommit        *Commit       `json:"last_commit"`
}

var statusJSONFields = []string{
	"location", "requested_location", "source", "is_repo", "message", "root", "branch", "detached",
	"head_short", "has_upstream", "upstream_branch", "ahead", "behind", "remote_url", "staged_count",
	"modified_count", "untracked_count", "deleted_count", "conflicted_count", "changed_files", "clean",
	"stash_count", "recent_commits", "last_commit",
}

func DecodeStatusPayload(payload map[string]any) (Status, error) {
	if len(payload) != len(statusJSONFields) {
		return Status{}, fmt.Errorf("git status response field inventory is not exact")
	}
	for _, field := range statusJSONFields {
		if _, ok := payload[field]; !ok {
			return Status{}, fmt.Errorf("git status response is missing %q", field)
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Status{}, fmt.Errorf("encode git status response: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var status Status
	if err := decoder.Decode(&status); err != nil {
		return Status{}, fmt.Errorf("decode git status response: %w", err)
	}
	if status.ChangedFiles == nil || status.RecentCommits == nil {
		return Status{}, fmt.Errorf("git status response inventories must be arrays")
	}
	if err := status.Validate(); err != nil {
		return Status{}, fmt.Errorf("validate git status response: %w", err)
	}
	return status, nil
}

func (status Status) Validate() error {
	if err := exactStatusString("location", status.Location, 4096, true); err != nil {
		return err
	}
	if status.Source != "core-local" && status.Source != "host-bridge" {
		return fmt.Errorf("git source %q is not registered", status.Source)
	}
	if status.RequestedLocation != "" {
		if err := exactStatusString("requested_location", status.RequestedLocation, 4096, true); err != nil {
			return err
		}
	}
	if !status.IsRepo {
		if err := exactStatusString("message", status.Message, 1024, true); err != nil {
			return err
		}
		if status.Root != "" || status.Branch != "" || status.HeadShort != "" || status.Message == "" || status.Detached || status.HasUpstream ||
			status.Ahead != 0 || status.Behind != 0 || len(status.ChangedFiles) != 0 || len(status.RecentCommits) != 0 ||
			status.LastCommit != nil || status.Clean {
			return fmt.Errorf("non-repository git status contains repository authority")
		}
		return nil
	}
	if status.Message != "" {
		return fmt.Errorf("repository git status must not contain a non-repository message")
	}
	for key, value := range map[string]string{"root": status.Root, "head_short": status.HeadShort} {
		if err := exactStatusString(key, value, 4096, true); err != nil {
			return err
		}
	}
	if status.Detached != (status.Branch == "") {
		return fmt.Errorf("git branch and detached authority contradict")
	}
	if status.Branch != "" {
		if err := exactStatusString("branch", status.Branch, 1024, true); err != nil {
			return err
		}
	}
	if status.HasUpstream != (status.UpstreamBranch != "") || (!status.HasUpstream && (status.Ahead != 0 || status.Behind != 0)) {
		return fmt.Errorf("git upstream authority contradicts its counters")
	}
	if status.UpstreamBranch != "" {
		if err := exactStatusString("upstream_branch", status.UpstreamBranch, 1024, true); err != nil {
			return err
		}
	}
	if status.RemoteURL != "" {
		if err := exactStatusString("remote_url", status.RemoteURL, 16*1024, true); err != nil {
			return err
		}
	}
	for _, value := range []int{status.Ahead, status.Behind, status.StagedCount, status.ModifiedCount, status.UntrackedCount,
		status.DeletedCount, status.ConflictedCount, status.StashCount} {
		if value < 0 {
			return fmt.Errorf("git status contains a negative counter")
		}
	}
	if len(status.ChangedFiles) > ChangedFileLimit || len(status.RecentCommits) > CommitLimit {
		return fmt.Errorf("git status exceeds its bounded inventory")
	}
	if status.Clean != (status.StagedCount+status.ModifiedCount+status.UntrackedCount+status.DeletedCount+status.ConflictedCount == 0) {
		return fmt.Errorf("git clean authority contradicts its counters")
	}
	for _, file := range status.ChangedFiles {
		if err := exactStatusString("changed file path", file.Path, 4096, true); err != nil {
			return err
		}
		if len(file.IndexStatus) != 1 || len(file.WorktreeStatus) != 1 || file.Status != file.IndexStatus+file.WorktreeStatus {
			return fmt.Errorf("git changed file status is malformed")
		}
	}
	for _, commit := range status.RecentCommits {
		if err := validateCommit(commit); err != nil {
			return err
		}
	}
	if len(status.RecentCommits) == 0 || status.LastCommit == nil || *status.LastCommit != status.RecentCommits[0] {
		return fmt.Errorf("git repository status requires exact last-commit authority")
	}
	return nil
}

func validateCommit(commit Commit) error {
	for key, value := range map[string]string{
		"commit hash": commit.Hash, "commit subject": commit.Subject,
		"commit author": commit.Author, "commit relative date": commit.RelativeDate,
	} {
		if err := exactStatusString(key, value, 16*1024, true); err != nil {
			return err
		}
	}
	return nil
}

func exactStatusString(label, value string, maxBytes int, nonblank bool) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || len(value) > maxBytes ||
		(nonblank && strings.TrimSpace(value) == "") {
		return fmt.Errorf("git %s must be one bounded exact UTF-8 string", label)
	}
	return nil
}
