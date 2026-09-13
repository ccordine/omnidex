package experiment

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestInputArchivePreservesExactBytesAndPermissions(t *testing.T) {
	files := []File{{Path: "nested/value.bin", Content: []byte{0, 1, '\r', '\n', 255}, Mode: 0o640}}
	var encoded bytes.Buffer
	if err := writeInputArchive(&encoded, files); err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(&encoded)
	root, err := reader.Next()
	if err != nil || root.Name != "workspace/" || root.Typeflag != tar.TypeDir || root.Uid != containerUserID {
		t.Fatalf("invalid experiment root: %+v %v", root, err)
	}
	parent, err := reader.Next()
	if err != nil || parent.Name != "workspace/nested/" || parent.Typeflag != tar.TypeDir || parent.Uid != containerUserID {
		t.Fatalf("invalid code-owned directory: %+v %v", parent, err)
	}
	file, err := reader.Next()
	if err != nil || file.Name != "workspace/"+files[0].Path || file.Mode != int64(files[0].Mode) || file.Uid != containerUserID || file.Gid != containerUserID {
		t.Fatalf("input file metadata changed: %+v %v", file, err)
	}
	content, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(content, files[0].Content) {
		t.Fatalf("input changed: %v %v", content, err)
	}
}

func TestCollectionRejectsUnobservedOrUnsafeArtifactIdentity(t *testing.T) {
	for _, defect := range []string{"missing", "link", "linked_parent", "duplicate", "traversal", "oversized", "permissions"} {
		t.Run(defect, func(t *testing.T) {
			headers := []*tar.Header{{Name: "workspace/", Typeflag: tar.TypeDir, Mode: 0o755}}
			file := &tar.Header{Name: "workspace/result.txt", Typeflag: tar.TypeReg, Mode: 0o644}
			collect := []string{"result.txt"}
			switch defect {
			case "missing":
				file.Name = "workspace/unrequested.txt"
			case "link":
				file.Typeflag, file.Linkname = tar.TypeSymlink, "/etc/passwd"
			case "linked_parent":
				headers = append(headers, &tar.Header{Name: "workspace/nested", Typeflag: tar.TypeSymlink, Linkname: "/etc", Mode: 0o777})
				file.Name, collect = "workspace/nested/result.txt", []string{"nested/result.txt"}
			case "duplicate":
				headers = append(headers, file)
			case "traversal":
				file.Name = "workspace/../result.txt"
			case "oversized":
				file.Size = MaxFileBytes + 1
			case "permissions":
				file.Mode = 0o4644
			}
			headers = append(headers, file)
			var stream bytes.Buffer
			writer := tar.NewWriter(&stream)
			for _, header := range headers {
				if err := writer.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
			}
			if defect != "oversized" {
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := readCollectedArchive(&stream, collect); err == nil {
				t.Fatal("unsafe artifact archive was accepted")
			}
		})
	}
}

func TestCollectionReturnsOnlyDeclaredFiles(t *testing.T) {
	var stream bytes.Buffer
	writer := tar.NewWriter(&stream)
	for _, header := range []*tar.Header{
		{Name: "workspace/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "workspace/unrelated", Typeflag: tar.TypeSymlink, Linkname: "/tmp", Mode: 0o777},
		{Name: "workspace/result.txt", Typeflag: tar.TypeReg, Mode: 0o640, Size: 5},
	} {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := writer.Write([]byte("exact")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := readCollectedArchive(&stream, []string{"result.txt"})
	if err != nil || len(files) != 1 || files[0].Path != "result.txt" || string(files[0].Content) != "exact" || files[0].Mode != 0o640 {
		t.Fatalf("collection changed exact declared output: %+v %v", files, err)
	}
}

func TestInvalidExperimentNeverInvokesDocker(t *testing.T) {
	for _, defect := range []string{"image", "path", "overlap", "mode", "environment", "timeout", "nul"} {
		t.Run(defect, func(t *testing.T) {
			request := Request{ImageID: "sha256:" + strings.Repeat("a", 64), Argv: []string{"true"}, Timeout: time.Second}
			switch defect {
			case "image":
				request.ImageID = "alpine:latest"
			case "path":
				request.Collect = []string{"../outside"}
			case "overlap":
				request.Input = []File{{Path: "a", Mode: 0o644}, {Path: "a/b", Mode: 0o644}}
			case "mode":
				request.Input = []File{{Path: "a", Mode: 0o4644}}
			case "environment":
				request.Environment = []string{"A=first", "A=second"}
			case "timeout":
				request.Timeout = 0
			case "nul":
				request.Argv = []string{"echo", "\x00"}
			}
			calls := 0
			docker := &Docker{cli: func(context.Context, []string, io.Reader, io.Writer, io.Writer) error { calls++; return nil }}
			if _, err := docker.Run(context.Background(), request); err == nil || calls != 0 {
				t.Fatalf("invalid request performed %d Docker operations: %v", calls, err)
			}
		})
	}
}
