package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Cleanup retires only receipts belonging to accepted items of this job.
// Two distinct outputs per build family retain the latest build and rollback;
// installed images, unfinished work, modified files and shared objects survive.
func (scope Scope) Cleanup(ctx context.Context) error {
	records, err := scope.records()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	retained := map[string]int{}
	var problems error
	engines := map[string]map[string]bool{}
	engineErrors := map[string]error{}
	for _, record := range records {
		if record.Job != scope.Job {
			continue
		}
		identity := record.Kind + "\x00" + strings.Join(record.Engine, "\x00") + "\x00" + record.Path
		if seen[identity] {
			_ = os.Remove(record.receipt)
			continue
		}
		seen[identity] = true
		if _, err := os.Stat(scope.completion(record.Task)); err != nil {
			continue
		}
		if record.Kind == "file" {
			info, err := os.Lstat(record.Path)
			if os.IsNotExist(err) {
				_ = os.Remove(record.receipt)
				continue
			}
			if err != nil {
				problems = errors.Join(problems, err)
				continue
			}
			if !info.Mode().IsRegular() || info.Size() != record.Size || !info.ModTime().Equal(record.Modified) {
				_ = os.Remove(record.receipt)
				continue
			}
		}
		family := record.Kind + ":" + record.Family
		if record.Kind != "container" && !record.Partial && retained[family] < 2 {
			retained[family]++
			continue
		}
		engineKey := strings.Join(record.Engine, "\x00")
		if record.Kind == "image" || record.ID != "" || record.Kind == "container" {
			if _, ok := engines[engineKey]; !ok {
				engines[engineKey], engineErrors[engineKey] = scope.containerReferences(ctx, record.Engine)
			}
			if err := engineErrors[engineKey]; err != nil {
				problems = errors.Join(problems, err)
				continue
			}
			if engines[engineKey][record.ID] || engines[engineKey][record.Path] {
				continue
			}
		}
		var removeErr error
		switch record.Kind {
		case "file":
			if runningExecutable(record.Path) {
				continue
			}
			removeErr = scope.RemoveFile(record)
		case "image":
			data, err := scope.DockerOutput(ctx, record.Engine, "image", "inspect", record.Path)
			if err != nil {
				continue
			}
			var found []struct {
				ID     string
				Config struct{ Labels map[string]string }
			}
			if json.Unmarshal(data, &found) != nil || len(found) != 1 || found[0].ID != record.ID || found[0].Config.Labels[ownerLabel] != key(scope.Workspace) {
				continue
			}
			_, removeErr = scope.DockerOutput(ctx, record.Engine, "image", "rm", record.Path)
		case "container":
			data, err := scope.DockerOutput(ctx, record.Engine, "container", "inspect", record.ID)
			if err != nil {
				continue
			}
			var found []struct {
				ID     string
				State  struct{ Running bool }
				Mounts []json.RawMessage
				Config struct{ Labels map[string]string }
			}
			if json.Unmarshal(data, &found) != nil || len(found) != 1 || found[0].ID != record.ID || found[0].State.Running || len(found[0].Mounts) != 0 || found[0].Config.Labels[ownerLabel] != key(scope.Workspace) {
				continue
			}
			_, removeErr = scope.DockerOutput(ctx, record.Engine, "container", "rm", record.ID)
		}
		if removeErr != nil {
			problems = errors.Join(problems, removeErr)
			continue
		}
		_ = os.Remove(record.receipt)
	}
	return problems
}

func (scope Scope) containerReferences(ctx context.Context, engine []string) (map[string]bool, error) {
	data, err := scope.DockerOutput(ctx, engine, "container", "ls", "-aq", "--no-trunc")
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	ids := strings.Fields(string(data))
	for _, id := range ids {
		data, err := scope.DockerOutput(ctx, engine, "container", "inspect", id)
		if err != nil {
			return nil, err
		}
		var found []struct {
			Image  string
			Config struct{ Image string }
		}
		if err := json.Unmarshal(data, &found); err != nil {
			return nil, err
		}
		for _, container := range found {
			result[container.Image] = true
			result[container.Config.Image] = true
		}
	}
	return result, nil
}

func runningExecutable(name string) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if executable, err := os.Readlink(filepath.Join("/proc", entry.Name(), "exe")); err == nil && executable == name {
			return true
		}
	}
	return false
}
