package hostbridge

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func (c *Client) Browse(ctx context.Context, path string, opts BrowseOptions) (*BrowseResult, error) {
	if err := validateBrowseBounds(opts); err != nil {
		return nil, err
	}
	query := url.Values{}
	if strings.TrimSpace(path) != "" {
		query.Set("path", strings.TrimSpace(path))
	}
	query.Set("limit", fmt.Sprint(opts.Limit))
	query.Set("offset", fmt.Sprint(opts.Offset))
	if opts.DirectoriesOnly {
		query.Set("directories_only", "true")
	}
	if strings.TrimSpace(opts.RequiredRoot) != "" {
		query.Set("required_root", strings.TrimSpace(opts.RequiredRoot))
	}
	payload, err := c.getJSON(ctx, "/v1/browse?"+query.Encode())
	if err != nil {
		return nil, err
	}
	limit, err := requiredIntField(payload, "limit")
	if err != nil {
		return nil, err
	}
	offset, err := requiredIntField(payload, "offset")
	if err != nil {
		return nil, err
	}
	nextOffset, err := requiredIntField(payload, "next_offset")
	if err != nil {
		return nil, err
	}
	previousOffset, err := requiredIntField(payload, "previous_offset")
	if err != nil {
		return nil, err
	}
	hasPrevious, err := requiredBoolField(payload, "has_previous")
	if err != nil {
		return nil, err
	}
	hasMore, err := requiredBoolField(payload, "has_more")
	if err != nil {
		return nil, err
	}
	result := &BrowseResult{
		Path: stringField(payload, "path"), Parent: stringField(payload, "parent"), Entries: []Entry{},
		Limit: limit, Offset: offset, HasPrevious: hasPrevious, PreviousOffset: previousOffset,
		HasMore: hasMore, NextOffset: nextOffset, RequiredRoot: stringField(payload, "required_root"),
	}
	if result.Limit != opts.Limit || result.Offset != opts.Offset {
		return nil, fmt.Errorf("host bridge browse response page does not match the request")
	}
	if required := strings.TrimSpace(opts.RequiredRoot); required != "" && strings.TrimSpace(result.RequiredRoot) == "" {
		return nil, fmt.Errorf("host bridge browse response did not attest required-root enforcement")
	}
	if result.HasMore && result.NextOffset <= result.Offset {
		return nil, fmt.Errorf("host bridge browse response has an invalid next offset")
	}
	if result.HasPrevious != (result.Offset > 0) || result.PreviousOffset < 0 || result.PreviousOffset >= result.Offset && result.HasPrevious {
		return nil, fmt.Errorf("host bridge browse response has an invalid previous offset")
	}
	entryObjects, err := browseEntryObjects(payload["entries"])
	if err != nil {
		return nil, err
	}
	for index, item := range entryObjects {
		name, err := requiredStringField(item, "name")
		if err != nil {
			return nil, fmt.Errorf("host bridge browse entry %d: %w", index, err)
		}
		path, err := requiredStringField(item, "path")
		if err != nil {
			return nil, fmt.Errorf("host bridge browse entry %d: %w", index, err)
		}
		isDir, err := requiredBoolField(item, "is_dir")
		if err != nil {
			return nil, fmt.Errorf("host bridge browse entry %d: %w", index, err)
		}
		result.Entries = append(result.Entries, Entry{
			Name: name, Path: path, IsDir: isDir,
		})
	}
	if len(result.Entries) > result.Limit {
		return nil, fmt.Errorf("host bridge browse response exceeds the requested page size")
	}
	return result, nil
}

func browseEntryObjects(raw any) ([]map[string]any, error) {
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("host bridge browse response entries must be an array")
	}
	entries := make([]map[string]any, 0, len(values))
	for index, value := range values {
		entry, ok := value.(map[string]any)
		if !ok || entry == nil {
			return nil, fmt.Errorf("host bridge browse response entry %d must be an object", index)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
