package api

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/hostbridge"
)

const (
	maxScrumModalRawQuerySize = 4 * 1024
	maxScrumModalFilePathSize = 4 * 1024
)

type scrumModalTab string

const (
	scrumModalTabCard    scrumModalTab = "card"
	scrumModalTabFiles   scrumModalTab = "files"
	scrumModalTabTests   scrumModalTab = "tests"
	scrumModalTabChannel scrumModalTab = "channel"
)

type scrumModalQuery struct {
	ProjectID  int64
	Tab        scrumModalTab
	FilePath   string
	FileOffset int
}

type scrumFilePageQuery struct {
	ProjectID  int64
	FilePath   string
	FileOffset int
}

func decodeScrumModalQuery(request *http.Request) (scrumModalQuery, error) {
	values, err := decodeExactScrumFileQuery(
		request, "Scrum card modal", "project_id", "tab", "file_path", "file_offset",
	)
	if err != nil {
		return scrumModalQuery{}, err
	}
	projectID, err := decodeScrumModalProjectID(values, "Scrum card modal")
	if err != nil {
		return scrumModalQuery{}, err
	}
	query := scrumModalQuery{ProjectID: projectID, Tab: scrumModalTabCard}
	if raw, present := oneQueryValue(values, "tab"); present {
		switch scrumModalTab(raw) {
		case scrumModalTabCard, scrumModalTabFiles, scrumModalTabTests, scrumModalTabChannel:
			query.Tab = scrumModalTab(raw)
		default:
			return scrumModalQuery{}, fmt.Errorf("Scrum card modal tab must be one exact registered value")
		}
	}
	rawPath, pathPresent := oneQueryValue(values, "file_path")
	rawOffset, offsetPresent := oneQueryValue(values, "file_offset")
	if query.Tab != scrumModalTabFiles {
		if pathPresent || offsetPresent {
			return scrumModalQuery{}, fmt.Errorf("Scrum card modal file state is accepted only for the files tab")
		}
		return query, nil
	}
	if !pathPresent {
		return scrumModalQuery{}, fmt.Errorf("Scrum card modal files tab requires explicit file_path; empty denotes the project root")
	}
	if err := validateScrumModalFilePath(rawPath); err != nil {
		return scrumModalQuery{}, err
	}
	query.FilePath = rawPath
	if offsetPresent {
		query.FileOffset, err = decodeScrumModalFileOffset(rawOffset)
		if err != nil {
			return scrumModalQuery{}, err
		}
	}
	return query, nil
}

func decodeScrumFilePageQuery(request *http.Request) (scrumFilePageQuery, error) {
	values, err := decodeExactScrumFileQuery(
		request, "Scrum card file page", "project_id", "file_path", "file_offset",
	)
	if err != nil {
		return scrumFilePageQuery{}, err
	}
	projectID, err := decodeScrumModalProjectID(values, "Scrum card file page")
	if err != nil {
		return scrumFilePageQuery{}, err
	}
	rawPath, pathPresent := oneQueryValue(values, "file_path")
	if !pathPresent {
		return scrumFilePageQuery{}, fmt.Errorf("Scrum card file page requires explicit file_path; empty denotes the project root")
	}
	if err := validateScrumModalFilePath(rawPath); err != nil {
		return scrumFilePageQuery{}, err
	}
	rawOffset, offsetPresent := oneQueryValue(values, "file_offset")
	if !offsetPresent {
		return scrumFilePageQuery{}, fmt.Errorf("Scrum card file page requires file_offset")
	}
	offset, err := decodeScrumModalFileOffset(rawOffset)
	if err != nil {
		return scrumFilePageQuery{}, err
	}
	return scrumFilePageQuery{ProjectID: projectID, FilePath: rawPath, FileOffset: offset}, nil
}

func decodeExactScrumFileQuery(
	request *http.Request,
	source string,
	allowed ...string,
) (url.Values, error) {
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("%s request URL is required", source)
	}
	if len(request.URL.RawQuery) > maxScrumModalRawQuerySize {
		return nil, fmt.Errorf("%s query exceeds the %d-byte bound", source, maxScrumModalRawQuerySize)
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("decode %s query: %w", source, err)
	}
	accepted := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		accepted[field] = struct{}{}
	}
	for field, items := range values {
		if _, ok := accepted[field]; !ok {
			return nil, fmt.Errorf("%s has unknown query field %q", source, field)
		}
		if len(items) != 1 {
			return nil, fmt.Errorf("%s query field %q must occur exactly once", source, field)
		}
	}
	return values, nil
}

func decodeScrumModalProjectID(values url.Values, source string) (int64, error) {
	raw, present := oneQueryValue(values, "project_id")
	if !present {
		return 0, fmt.Errorf("%s requires project_id", source)
	}
	projectID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || projectID <= 0 || strconv.FormatInt(projectID, 10) != raw {
		return 0, fmt.Errorf("%s project_id must be one canonical positive integer", source)
	}
	return projectID, nil
}

func decodeScrumModalFileOffset(raw string) (int, error) {
	offset, err := strconv.Atoi(raw)
	if err != nil || strconv.Itoa(offset) != raw || offset < 0 || offset > hostbridge.MaxBrowseOffset {
		return 0, fmt.Errorf(
			"Scrum card file_offset must be one canonical integer between 0 and %d",
			hostbridge.MaxBrowseOffset,
		)
	}
	return offset, nil
}

func validateScrumModalFilePath(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("Scrum card file_path must be valid UTF-8")
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("Scrum card file_path must not contain NUL")
	}
	if len(value) > maxScrumModalFilePathSize {
		return fmt.Errorf("Scrum card file_path exceeds the %d-byte bound", maxScrumModalFilePathSize)
	}
	if value == "" {
		return nil
	}
	if strings.Contains(value, "\\") || path.IsAbs(value) || filepath.IsAbs(filepath.FromSlash(value)) ||
		filepath.VolumeName(filepath.FromSlash(value)) != "" || path.Clean(value) != value {
		return fmt.Errorf("Scrum card file_path must be an exact canonical relative path or empty root")
	}
	return nil
}
