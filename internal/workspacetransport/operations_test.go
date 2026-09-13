package workspacetransport

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/workspace"
)

func TestWorkspaceOperationsRejectUnrelatedAndUnboundedFields(t *testing.T) {
	for _, req := range []request{
		{ID: 1, Kind: opAttest, Path: "known"},
		{ID: 1, Kind: opAcquire, Lease: 3},
		{ID: 1, Kind: opReadFile, Path: "../outside"},
		{ID: 1, Kind: opReadDirectory, Path: ".", Limit: workspace.MaxDirectoryPageEntries + 1},
		{ID: 1, Kind: opReadDirectory, Path: ".", Limit: 1, After: "../outside"},
		{ID: 1, Kind: opPrepare, Lease: 2, DesiredCount: workspace.MaxReconciliationFiles + 1},
		{ID: 1, Kind: opApply, Lease: 2},
		{ID: 1, Kind: opRelease, Lease: 2, Prepared: 3},
	} {
		if err := req.validate(); err == nil {
			t.Errorf("accepted invalid request: %#v", req)
		}
	}
	request := request{ID: 1, Kind: opReadDirectory, Path: "src", Limit: 1}
	for _, page := range []workspace.DirectoryPage{
		{HasMore: true},
		{Entries: []workspace.Entry{{Path: "outside/file", Kind: workspace.EntryFile}}},
		{Entries: []workspace.Entry{{Path: "src/a", Kind: workspace.EntryFile}, {Path: "src/b", Kind: workspace.EntryFile}}},
	} {
		if err := (response{ID: 1, Page: &page}).validate(request); err == nil {
			t.Errorf("accepted invalid page: %#v", page)
		}
	}
	for _, size := range []int64{-1, workspace.MaxReconciliationFileBytes + 1} {
		total := int64(0)
		if err := admitContentBytes(&total, size); err == nil {
			t.Errorf("accepted invalid content size %d", size)
		}
	}
	total := int64(workspace.MaxReconciliationTotalBytes)
	if err := admitContentBytes(&total, 1); err == nil {
		t.Fatal("accepted excess aggregate content")
	}
}

func TestDirectoryMetadataBoundIncludesMaximumEscapedPaths(t *testing.T) {
	parent := strings.Repeat(strings.Repeat("\x01", 240)+"/", 16) + strings.Repeat("\x01", 180)
	page := workspace.DirectoryPage{}
	for index := range workspace.MaxDirectoryPageEntries {
		page.Entries = append(page.Entries, workspace.Entry{Path: parent + fmt.Sprintf("/entry%03d", index), Kind: workspace.EntryFile})
	}
	req := request{ID: 1, Kind: opReadDirectory, Path: parent, Limit: workspace.MaxDirectoryPageEntries}
	reply := response{ID: 1, Page: &page}
	if err := req.validate(); err != nil {
		t.Fatal(err)
	}
	if err := reply.validate(req); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxFrameBytes {
		t.Fatalf("valid directory metadata requires %d bytes, transport allows %d", len(data), MaxFrameBytes)
	}
}

func TestChangeProjectionRejectsEmptyPathsAndUnrequestedMutations(t *testing.T) {
	prepared := &prepared{desired: map[string]workspace.DesiredFile{"known": {Path: "known", Present: true}}}
	for _, change := range []workspace.Change{
		{Kind: workspace.ChangeDelete},
		{Path: ".", Kind: workspace.ChangeDelete},
		{Path: "unrelated", Kind: workspace.ChangeDelete},
		{Path: "known", Kind: workspace.ChangeMove, SourcePath: "unrelated"},
	} {
		if err := prepared.validateChange(change, map[string]bool{}); err == nil {
			t.Errorf("accepted unrequested change: %#v", change)
		}
	}
}
