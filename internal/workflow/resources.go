// Package workflow owns reusable build locations and the lifetime of resources
// created through a native workflow. It never scans or prunes shared storage.
package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Scope struct {
	Directory        string `json:"directory"`
	Workspace        string `json:"workspace"`
	Job              int64  `json:"job"`
	Task             string `json:"task"`
	Docker           string `json:"docker,omitempty"`
	Go               string `json:"go,omitempty"`
	DockerContext    string `json:"docker_context,omitempty"`
	NoDockerTracking bool   `json:"no_docker_tracking,omitempty"`
}

type Resource struct {
	Kind, Family, ID, Path string
	Engine                 []string
	Job                    int64
	Task                   string
	Size                   int64
	Modified               time.Time
	Created                time.Time
	Generated              bool
	Partial                bool
	receipt                string
}

func key(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func Open(base, workspace string) (Scope, error) {
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return Scope{}, err
	}
	directory := filepath.Join(base, "bin", "agent-runtime", "workflows", key(root))
	for _, name := range []string{"", "records", "completed", "bin", "cache"} {
		p := filepath.Join(directory, name)
		if err := os.MkdirAll(p, 0700); err != nil {
			return Scope{}, err
		}
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Scope{}, fmt.Errorf("workflow storage is not a real directory")
		}
	}
	return Scope{Directory: directory, Workspace: root}, nil
}

func (scope Scope) Record(resource Resource) error {
	resource.Job, resource.Task, resource.Created = scope.Job, scope.Task, time.Now().UTC()
	file, err := os.CreateTemp(filepath.Join(scope.Directory, "records"), ".record-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := json.NewEncoder(file).Encode(resource); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, name+".json")
}

func (scope Scope) Complete(tasks []string) error {
	for _, task := range tasks {
		if err := os.WriteFile(scope.completion(task), []byte("accepted\n"), 0600); err != nil {
			return err
		}
	}
	return nil
}

func (scope Scope) completion(task string) string {
	return filepath.Join(scope.Directory, "completed", key(fmt.Sprintf("%d/%s", scope.Job, task)))
}

func (scope Scope) records() ([]Resource, error) {
	entries, err := os.ReadDir(filepath.Join(scope.Directory, "records"))
	if err != nil {
		return nil, err
	}
	var result []Resource
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(scope.Directory, "records", entry.Name()))
		if err != nil {
			return nil, err
		}
		var resource Resource
		if err := json.Unmarshal(data, &resource); err != nil {
			return nil, err
		}
		resource.receipt = filepath.Join(scope.Directory, "records", entry.Name())
		result = append(result, resource)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Created.After(result[j].Created) })
	return result, nil
}

func (scope Scope) ownsPath(name string) bool {
	if !filepath.IsAbs(name) {
		return false
	}
	for _, root := range []string{scope.Workspace, scope.Directory} {
		rel, err := filepath.Rel(root, name)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (scope Scope) TrackFile(name, family string, existed bool, imageID string, engines ...[]string) error {
	return scope.trackFile(name, family, existed, imageID, false, false, engines...)
}

// TrackOutput is used only for an observed Go build or Docker image export.
// Those commands can write outside the checkout; exact newly generated files
// are still attributable, unlike an ambient directory scan.
func (scope Scope) TrackOutput(name, family string, existed bool, imageID string, partial bool, engines ...[]string) error {
	return scope.trackFile(name, family, existed, imageID, true, partial, engines...)
}

func (scope Scope) trackFile(name, family string, existed bool, imageID string, generated, partial bool, engines ...[]string) error {
	name, err := filepath.Abs(name)
	if err != nil || (!scope.ownsPath(name) && !generated) {
		return err
	}
	if generated && !scope.ownsPath(name) {
		user, _ := os.UserHomeDir()
		for _, prefix := range []string{"/usr/", "/etc/", "/bin/", "/sbin/", "/opt/", filepath.Join(user, ".local", "bin") + string(os.PathSeparator), filepath.Join(user, ".config") + string(os.PathSeparator)} {
			if strings.HasPrefix(name, prefix) {
				return nil
			}
		}
	}
	if existed {
		// Existing outputs become owned only if a previous workflow created them.
		records, err := scope.records()
		if err != nil {
			return err
		}
		owned := false
		for _, record := range records {
			if record.Kind == "file" && record.Path == name {
				owned = true
				break
			}
		}
		if !owned {
			return nil
		}
	}
	info, err := os.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	var engine []string
	if len(engines) > 0 {
		engine = engines[0]
	}
	return scope.Record(Resource{Kind: "file", Family: family, Path: name, ID: imageID, Engine: engine, Generated: generated, Partial: partial, Size: info.Size(), Modified: info.ModTime()})
}

func (scope Scope) RemoveFile(record Resource) error {
	if !scope.ownsPath(record.Path) && !record.Generated {
		return nil
	}
	info, err := os.Lstat(record.Path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != record.Size || !info.ModTime().Equal(record.Modified) {
		return nil
	}
	// Resolve parents before unlinking; a changed symlink must not redirect cleanup.
	parent, err := filepath.EvalSymlinks(filepath.Dir(record.Path))
	if err != nil {
		return err
	}
	if filepath.Join(parent, filepath.Base(record.Path)) != record.Path {
		return nil
	}
	return os.Remove(record.Path)
}
