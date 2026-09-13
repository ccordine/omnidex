package workspacetransport

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/workspace"
)

type operation string

const (
	opAttest        operation = "attest"
	opStat          operation = "stat"
	opReadFile      operation = "read_file"
	opReadDirectory operation = "read_directory"
	opAcquire       operation = "acquire"
	opPrepare       operation = "prepare"
	opApply         operation = "apply"
	opRelease       operation = "release"
)

type request struct {
	ID            uint64    `json:"id"`
	Kind          operation `json:"kind"`
	Lease         uint64    `json:"lease,omitempty"`
	Prepared      uint64    `json:"prepared,omitempty"`
	Path          string    `json:"path,omitempty"`
	After         string    `json:"after,omitempty"`
	Limit         int       `json:"limit,omitempty"`
	DesiredCount  int       `json:"desired_count,omitempty"`
	ExpectedCount int       `json:"expected_count,omitempty"`
}

type requestData struct {
	request
	desired  []workspace.DesiredFile
	expected []workspace.File
}

type response struct {
	ID             uint64                   `json:"id"`
	Identity       string                   `json:"identity,omitempty"`
	Error          string                   `json:"error,omitempty"`
	ErrorKind      string                   `json:"error_kind,omitempty"`
	Entry          *workspace.Entry         `json:"entry,omitempty"`
	Page           *workspace.DirectoryPage `json:"page,omitempty"`
	Change         *workspace.Change        `json:"change,omitempty"`
	AppliedChanges int                      `json:"applied_changes,omitempty"`
}

type responseData struct {
	response
	content []byte
}

func (r request) validate() error {
	if r.ID == 0 {
		return fmt.Errorf("workspace request requires its exact sequence")
	}
	allowed := request{ID: r.ID, Kind: r.Kind}
	switch r.Kind {
	case opAcquire:
	case opAttest:
		allowed.Lease = r.Lease
	case opStat, opReadFile, opReadDirectory:
		allowed.Lease, allowed.Path = r.Lease, r.Path
		if err := (workspace.Entry{Path: r.Path, Kind: workspace.EntryFile}).Validate(r.Path); err != nil {
			return err
		}
		if r.Kind == opReadDirectory {
			allowed.After, allowed.Limit = r.After, r.Limit
			if r.Limit < 1 || r.Limit > workspace.MaxDirectoryPageEntries ||
				(r.After != "" && (r.After == "." || r.After == ".." || strings.ContainsAny(r.After, "/\\\x00"))) {
				return fmt.Errorf("workspace directory request has invalid pagination")
			}
		}
	case opPrepare, opApply, opRelease:
		if r.Lease == 0 {
			return fmt.Errorf("workspace mutation requires an acquired lease")
		}
		allowed.Lease = r.Lease
		if r.Kind == opPrepare {
			allowed.DesiredCount, allowed.ExpectedCount = r.DesiredCount, r.ExpectedCount
			if r.DesiredCount < 0 || r.ExpectedCount < 0 || r.DesiredCount > workspace.MaxReconciliationFiles || r.ExpectedCount > workspace.MaxReconciliationFiles {
				return fmt.Errorf("workspace preparation exceeds its file count bound")
			}
		}
		if r.Kind == opApply {
			allowed.Prepared = r.Prepared
			if r.Prepared == 0 {
				return fmt.Errorf("workspace apply requires its exact preparation")
			}
		}
	default:
		return fmt.Errorf("workspace request has an unknown operation %q", r.Kind)
	}
	if r != allowed {
		return fmt.Errorf("workspace request contains unrelated operation fields")
	}
	return nil
}

func (r response) validate(req request) error {
	allowed := response{ID: r.ID, Identity: r.Identity, Error: r.Error, ErrorKind: r.ErrorKind}
	if r.ID != req.ID {
		return fmt.Errorf("workspace response sequence differs from request")
	}
	if r.Error != "" {
		switch r.ErrorKind {
		case "failure", "missing", "busy":
		default:
			return fmt.Errorf("workspace response has an unknown error kind")
		}
	} else {
		if r.ErrorKind != "" {
			return fmt.Errorf("workspace response has an error kind without an error")
		}
		switch req.Kind {
		case opStat, opReadFile:
			allowed.Entry = r.Entry
			if r.Entry == nil {
				return fmt.Errorf("workspace response lacks its requested entry")
			}
			if err := r.Entry.Validate(req.Path); err != nil {
				return err
			}
			if req.Kind == opReadFile && (r.Entry.Kind != workspace.EntryFile || r.Entry.Size > workspace.MaxReconciliationFileBytes) {
				return fmt.Errorf("workspace response is not a bounded regular file")
			}
		case opReadDirectory:
			allowed.Page = r.Page
			if err := validatePage(req, r.Page); err != nil {
				return err
			}
		case opApply:
			allowed.Change = r.Change
		}
	}
	if req.Kind == opApply {
		allowed.AppliedChanges = r.AppliedChanges
		if r.AppliedChanges < 0 || (r.Change != nil && r.AppliedChanges != 0) {
			return fmt.Errorf("workspace response has an invalid applied change count")
		}
	}
	if r != allowed {
		return fmt.Errorf("workspace response contains unrelated operation fields")
	}
	return nil
}

func validatePage(req request, page *workspace.DirectoryPage) error {
	if page == nil || len(page.Entries) > req.Limit || (page.HasMore && len(page.Entries) != req.Limit) {
		return fmt.Errorf("workspace directory page exceeds its requested bound")
	}
	previous := req.After
	for _, entry := range page.Entries {
		if err := entry.Validate(entry.Path); err != nil {
			return err
		}
		name := path.Base(entry.Path)
		if path.Dir(entry.Path) != req.Path || name <= previous || name == "." {
			return fmt.Errorf("workspace page is not ordered within its requested directory")
		}
		previous = name
	}
	return nil
}

type remoteError struct{ kind, message string }

func (e *remoteError) Error() string { return "client workspace: " + e.message }
func (e *remoteError) Is(target error) bool {
	return e.kind == "missing" && target == os.ErrNotExist || e.kind == "busy" && target == workspace.ErrWorkspaceBusy
}
func setResponseError(r *response, err error) {
	if err == nil {
		return
	}
	r.Error, r.ErrorKind = err.Error(), "failure"
	if errors.Is(err, os.ErrNotExist) {
		r.ErrorKind = "missing"
	}
	if errors.Is(err, workspace.ErrWorkspaceBusy) {
		r.ErrorKind = "busy"
	}
}
