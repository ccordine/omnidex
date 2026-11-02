package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const ownerLabel = "io.omnidex.workflow"

func goFamily(args []string) string {
	valued := map[string]bool{"-o": true, "-ldflags": true, "-gcflags": true, "-asmflags": true, "-tags": true, "-mod": true, "-modfile": true, "-overlay": true, "-p": true, "-pkgdir": true, "-toolexec": true, "-buildmode": true, "-compiler": true, "-coverpkg": true, "-coverprofile": true}
	packages := []string{args[0]}
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			if valued[args[i]] && i+1 < len(args) {
				i++
			}
			continue
		}
		packages = append(packages, args[i])
	}
	cwd, _ := os.Getwd()
	goos, goarch := os.Getenv("GOOS"), os.Getenv("GOARCH")
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	flavor := option(args, "-buildmode") + "\x00" + option(args, "-tags") + "\x00" + os.Getenv("CGO_ENABLED")
	for _, arg := range args {
		if arg == "-race" || arg == "-msan" || arg == "-asan" {
			flavor += "\x00" + arg
		}
	}
	return "go:" + key(cwd+"\x00"+goos+"/"+goarch+"\x00"+flavor+"\x00"+strings.Join(packages, "\x00"))
}

func dockerBuildContext(args []string) string {
	valued := map[string]bool{"-t": true, "--tag": true, "-f": true, "--file": true, "--build-arg": true, "--build-context": true, "--cache-from": true, "--cache-to": true, "--cgroup-parent": true, "--iidfile": true, "--label": true, "--network": true, "--output": true, "-o": true, "--platform": true, "--progress": true, "--secret": true, "--shm-size": true, "--ssh": true, "--target": true, "--ulimit": true, "--builder": true, "--metadata-file": true, "--attest": true, "--provenance": true, "--sbom": true}
	var target string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if valued[arg] && i+1 < len(args) {
				i++
			}
			continue
		}
		target = arg
	}
	return target
}

func option(args []string, names ...string) string {
	for i, arg := range args {
		for _, name := range names {
			if arg == name && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(arg, name+"=") {
				return strings.TrimPrefix(arg, name+"=")
			}
		}
	}
	return ""
}

// Docker prefixes are retained with each record; cleanup uses the same engine.
func dockerCommand(args []string) ([]string, []string) {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		arg := strings.SplitN(args[i], "=", 2)[0]
		if arg != "--context" && arg != "-c" {
			return nil, nil
		}
		if !strings.Contains(args[i], "=") {
			i++
		}
		i++
	}
	if i >= len(args) {
		return nil, nil
	}
	return args[:i], args[i:]
}

func (scope Scope) DockerOutput(ctx context.Context, engine []string, args ...string) ([]byte, error) {
	if scope.Docker == "" {
		var err error
		scope.Docker, err = exec.LookPath("docker")
		if err != nil {
			return nil, err
		}
	}
	return exec.CommandContext(ctx, scope.Docker, append(append([]string{}, engine...), args...)...).Output()
}

// Execute preserves the ordinary CLI, its streams and exit status. The adapter
// only labels native-created Docker objects and records exact generated outputs.
func Execute(ctx context.Context, scope Scope, tool string, args []string, in io.Reader, out, stderr io.Writer) error {
	binary := scope.Go
	if tool == "docker" {
		binary = scope.Docker
	} else if tool != "go" {
		return fmt.Errorf("unknown workflow command")
	}
	if binary == "" {
		return fmt.Errorf("workflow command is unavailable")
	}
	original := append([]string(nil), args...)
	var target, family string
	var engine, command []string
	var build, container bool
	var imageID string
	if tool == "go" && len(args) > 0 && (args[0] == "build" || args[0] == "test") {
		target = option(args, "-o")
		family = goFamily(args)
	}
	if tool == "docker" && !scope.NoDockerTracking {
		if scope.DockerContext != "" && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			args = append([]string{"--context", scope.DockerContext}, args...)
		}
		engine, command = dockerCommand(args)
		if len(command) > 1 && ((command[0] == "image" && command[1] == "build") || (command[0] == "buildx" && command[1] == "build") || command[0] == "container") {
			command = command[1:]
		}
		if len(command) > 0 {
			build = command[0] == "build"
			container = command[0] == "create" || command[0] == "run"
			if build || container {
				prefix := len(args) - len(command) + 1
				labels := []string{"--label", ownerLabel + "=" + key(scope.Workspace)}
				if container {
					labels = append(labels, "--label", "io.omnidex.job="+fmt.Sprint(scope.Job), "--label", "io.omnidex.task="+key(scope.Task))
				}
				args = append(append(append([]string{}, args[:prefix]...), labels...), args[prefix:]...)
			}
			if command[0] == "save" || (command[0] == "image" && len(command) > 1 && command[1] == "save") {
				target = option(command, "-o", "--output")
			}
		}
	}
	if target != "" && !filepath.IsAbs(target) {
		cwd, _ := os.Getwd()
		target = filepath.Join(cwd, target)
	}
	_, beforeErr := os.Lstat(target)
	existed := target != "" && !os.IsNotExist(beforeErr)
	process := exec.CommandContext(ctx, binary, args...)
	process.Stdin = in
	process.Stdout = out
	process.Stderr = stderr
	// Child tools retain the normal host/cache environment and original binary.
	err := process.Run()
	if tool == "docker" && build {
		tags := []string{}
		for i, arg := range command {
			if (arg == "-t" || arg == "--tag") && i+1 < len(command) {
				tags = append(tags, command[i+1])
			}
			if strings.HasPrefix(arg, "--tag=") {
				tags = append(tags, strings.TrimPrefix(arg, "--tag="))
			}
		}
		cwd, _ := os.Getwd()
		family = "image:" + key(cwd+"\x00"+strings.Join(engine, "\x00")+"\x00"+dockerBuildContext(command)+"\x00"+option(command, "-f", "--file")+"\x00"+option(command, "--target")+"\x00"+option(command, "--platform"))
		for _, tag := range tags {
			if data, e := scope.DockerOutput(context.WithoutCancel(ctx), engine, "image", "inspect", tag); e == nil {
				var images []struct {
					ID     string
					Config struct{ Labels map[string]string }
				}
				if json.Unmarshal(data, &images) == nil && len(images) == 1 && images[0].Config.Labels[ownerLabel] == key(scope.Workspace) {
					_ = scope.Record(Resource{Kind: "image", Family: family, ID: images[0].ID, Path: tag, Engine: engine, Partial: err != nil})
				}
			}
		}
	}
	if target != "" {
		if tool == "docker" {
			family = "archive:" + key(strings.Join(engine, "\x00"))
			records, _ := scope.records()
		findImage:
			for _, record := range records {
				if record.Kind == "image" && slices.Equal(record.Engine, engine) {
					for _, arg := range original {
						if arg == record.Path {
							family = "archive:" + record.Family
							imageID = record.ID
							break findImage
						}
					}
				}
			}
			if imageID == "" {
				family += ":" + key(target)
			}
		}
		if e := scope.TrackOutput(target, family, existed, imageID, err != nil, engine); e != nil {
			fmt.Fprintln(stderr, "omnidex: resource tracking remains pending:", e)
		}
	}
	if container {
		data, e := scope.DockerOutput(context.WithoutCancel(ctx), engine, "container", "ls", "-aq", "--no-trunc", "--filter", "label="+ownerLabel+"="+key(scope.Workspace), "--filter", "label=io.omnidex.job="+fmt.Sprint(scope.Job), "--filter", "label=io.omnidex.task="+key(scope.Task))
		if e == nil {
			for _, id := range strings.Fields(string(data)) {
				_ = scope.Record(Resource{Kind: "container", ID: id, Path: id, Family: "container", Engine: engine})
			}
		}
	}
	return err
}
