package experiment

import (
	"archive/tar"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestArtifactTreeCollectionRetainsOnlyExactSubtree(t *testing.T) {
	var stream bytes.Buffer
	input := []File{
		{Path: "reports/nested/value.bin", Content: []byte{0, 1, 255}, Mode: 0o640},
		{Path: "reports/index.txt", Content: []byte("index"), Mode: 0o644},
		{Path: "reportssibling/excluded.txt", Content: []byte("unrelated"), Mode: 0o600},
	}
	if err := writeInputArchive(&stream, input); err != nil {
		t.Fatal(err)
	}
	files, err := readArtifactArchive(&stream, nil, "reports")
	if err != nil || len(files) != 2 || files[0].Path != "reports/index.txt" || files[1].Path != "reports/nested/value.bin" || !bytes.Equal(files[1].Content, input[0].Content) || files[1].Mode != 0o640 {
		t.Fatalf("artifact tree differs from exact subtree: %+v %v", files, err)
	}
}

func TestArtifactTreeCollectionRejectsMissingLinkedOrExcessiveResults(t *testing.T) {
	for _, defect := range []string{"missing root", "linked root", "linked file", "linked parent", "file count"} {
		t.Run(defect, func(t *testing.T) {
			headers := []*tar.Header{
				{Name: "workspace/", Typeflag: tar.TypeDir, Mode: 0o755},
				{Name: "workspace/reports/", Typeflag: tar.TypeDir, Mode: 0o755},
			}
			switch defect {
			case "missing root":
				headers = headers[:1]
			case "linked root":
				headers[1].Typeflag, headers[1].Linkname = tar.TypeSymlink, "/tmp"
			case "linked file":
				headers = append(headers, &tar.Header{Name: "workspace/reports/result", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777})
			case "linked parent":
				headers = append(headers,
					&tar.Header{Name: "workspace/reports/nested", Typeflag: tar.TypeSymlink, Linkname: "/tmp", Mode: 0o777},
					&tar.Header{Name: "workspace/reports/nested/result", Typeflag: tar.TypeReg, Mode: 0o644})
			case "file count":
				for index := 0; index <= MaxFiles; index++ {
					headers = append(headers, &tar.Header{Name: fmt.Sprintf("workspace/reports/result%d", index), Typeflag: tar.TypeReg, Mode: 0o644})
				}
			}
			var stream bytes.Buffer
			writer := tar.NewWriter(&stream)
			for _, header := range headers {
				if err := writer.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := readArtifactArchive(&stream, nil, "reports"); err == nil {
				t.Fatal("invalid artifact tree accepted")
			} else if defect == "file count" && !strings.Contains(err.Error(), "file count bound") {
				t.Fatal(err)
			}
		})
	}
}
