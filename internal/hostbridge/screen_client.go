package hostbridge

import (
	"context"
	"fmt"
	"net/url"
)

func (c *Client) ScreenMonitors(ctx context.Context, request ScreenMonitorPageRequest) (ScreenMonitorPage, error) {
	if request.Limit < 1 || request.Limit > MaxScreenMonitorPageSize || request.Offset < 0 {
		return ScreenMonitorPage{}, fmt.Errorf("invalid screen monitor page request")
	}
	query := url.Values{"limit": {fmt.Sprint(request.Limit)}, "offset": {fmt.Sprint(request.Offset)}}
	payload, err := c.getJSON(ctx, "/v1/screen/monitors?"+query.Encode())
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	page, err := decodeScreenMonitorPage(payload)
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	if page.Limit != request.Limit || page.Offset != request.Offset {
		return ScreenMonitorPage{}, fmt.Errorf("host bridge monitor page does not match the request")
	}
	if len(page.Monitors) > page.Limit || page.HasPrevious != (page.Offset > 0) ||
		(page.HasPrevious && (page.PreviousOffset < 0 || page.PreviousOffset >= page.Offset)) ||
		(!page.HasPrevious && page.PreviousOffset != 0) ||
		(page.HasMore && (len(page.Monitors) == 0 || page.NextOffset != page.Offset+len(page.Monitors))) ||
		(!page.HasMore && page.NextOffset != 0) {
		return ScreenMonitorPage{}, fmt.Errorf("host bridge monitor page metadata is invalid")
	}
	return page, nil
}

func decodeScreenMonitorPage(payload map[string]any) (ScreenMonitorPage, error) {
	limit, err := requiredIntField(payload, "limit")
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	offset, err := requiredIntField(payload, "offset")
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	previous, err := requiredIntField(payload, "previous_offset")
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	next, err := requiredIntField(payload, "next_offset")
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	hasPrevious, err := requiredBoolField(payload, "has_previous")
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	hasMore, err := requiredBoolField(payload, "has_more")
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	page := ScreenMonitorPage{
		Backend: stringField(payload, "backend"), StreamPath: stringField(payload, "stream_path"),
		Limit: limit, Offset: offset, HasPrevious: hasPrevious, PreviousOffset: previous,
		HasMore: hasMore, NextOffset: next, Monitors: []ScreenMonitor{},
	}
	if page.Backend == "" || page.StreamPath != "/v1/screen/mjpeg" {
		return ScreenMonitorPage{}, fmt.Errorf("host bridge monitor page lacks its backend or stream path")
	}
	rawItems, ok := payload["monitors"].([]any)
	if !ok {
		return ScreenMonitorPage{}, fmt.Errorf("host bridge monitor page lacks monitors")
	}
	for index, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			return ScreenMonitorPage{}, fmt.Errorf("host bridge monitor %d must be an object", index)
		}
		monitor, err := decodeScreenMonitor(item, index)
		if err != nil {
			return ScreenMonitorPage{}, err
		}
		page.Monitors = append(page.Monitors, monitor)
	}
	return page, nil
}

func decodeScreenMonitor(item map[string]any, index int) (ScreenMonitor, error) {
	monitor := ScreenMonitor{ID: stringField(item, "id"), Name: stringField(item, "name")}
	var err error
	if monitor.Width, err = requiredIntField(item, "width"); err != nil {
		return ScreenMonitor{}, fmt.Errorf("monitor %d width: %w", index, err)
	}
	if monitor.Height, err = requiredIntField(item, "height"); err != nil {
		return ScreenMonitor{}, fmt.Errorf("monitor %d height: %w", index, err)
	}
	if monitor.X, err = requiredIntField(item, "x"); err != nil {
		return ScreenMonitor{}, fmt.Errorf("monitor %d x: %w", index, err)
	}
	if monitor.Y, err = requiredIntField(item, "y"); err != nil {
		return ScreenMonitor{}, fmt.Errorf("monitor %d y: %w", index, err)
	}
	if monitor.Primary, err = requiredBoolField(item, "primary"); err != nil {
		return ScreenMonitor{}, fmt.Errorf("monitor %d primary: %w", index, err)
	}
	if monitor.ID == "" || monitor.Name == "" {
		return ScreenMonitor{}, fmt.Errorf("monitor %d lacks id or name", index)
	}
	return monitor, nil
}
