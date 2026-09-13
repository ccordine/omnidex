package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUnboundCreateResponseNeverAuthorizesContainerRemoval(t *testing.T) {
	for _, mismatch := range []string{"name", "label"} {
		t.Run(mismatch, func(t *testing.T) {
			foreignID, ownedID := strings.Repeat("b", 64), strings.Repeat("c", 64)
			request := Request{ImageID: "sha256:" + strings.Repeat("a", 64), Argv: []string{"/bin/true"}, Timeout: time.Second}
			name, removed := "", false
			docker := &Docker{cli: func(_ context.Context, argv []string, _ io.Reader, stdout, _ io.Writer) error {
				switch argv[1] {
				case "create":
					name = argv[3]
					_, err := fmt.Fprintln(stdout, foreignID)
					return err
				case "inspect":
					if argv[len(argv)-1] != foreignID {
						t.Fatalf("inspected unexpected container: %v", argv)
					}
					observedName, label := "/"+name, name
					if mismatch == "name" {
						observedName = "/unrelated"
					} else {
						label = "unrelated"
					}
					return json.NewEncoder(stdout).Encode(map[string]any{
						"Id": foreignID, "Name": observedName, "Image": request.ImageID, "Path": "/bin/sh", "Args": []string{"-c", "exec sleep 2147483647"},
						"Config": map[string]any{
							"WorkingDir": WorkingDirectory, "User": "65532:65532", "Tty": false,
							"Labels": map[string]string{"com.omnidex.experiment": label},
						},
						"HostConfig": map[string]any{
							"NetworkMode": "none", "Privileged": false, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges"},
						},
						"Mounts":          []any{},
						"NetworkSettings": map[string]any{"Networks": map[string]any{"none": map[string]any{}}},
						"State":           map[string]any{"Status": "created", "Running": false, "OOMKilled": false, "ExitCode": 0, "Error": ""},
					})
				case "ls":
					if !slices.Contains(argv, "label=com.omnidex.experiment="+name) || !slices.Contains(argv, "name=^/"+name+"$") {
						t.Fatalf("cleanup omitted exact creation ownership: %v", argv)
					}
					if !removed {
						_, err := fmt.Fprintln(stdout, ownedID)
						return err
					}
					return nil
				case "rm":
					if argv[len(argv)-1] != ownedID {
						t.Fatalf("cleanup attempted to remove unowned container: %v", argv)
					}
					removed = true
					return nil
				default:
					t.Fatalf("unbound experiment continued: %v", argv)
					return nil
				}
			}}
			result, err := docker.Run(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), "exact experiment authority") || result.ContainerID != "" || !removed {
				t.Fatalf("result = %#v, error = %v, owned container removed = %t", result, err, removed)
			}
		})
	}
}
