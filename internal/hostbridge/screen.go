package hostbridge

type ScreenMonitor struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Primary bool   `json:"primary"`
}

const (
	DefaultScreenMonitorPageSize = 20
	MaxScreenMonitorPageSize     = 100
)

type ScreenMonitorPageRequest struct {
	Limit  int
	Offset int
}

type ScreenMonitorPage struct {
	Monitors       []ScreenMonitor `json:"monitors"`
	Backend        string          `json:"backend"`
	StreamPath     string          `json:"stream_path"`
	Limit          int             `json:"limit"`
	Offset         int             `json:"offset"`
	HasPrevious    bool            `json:"has_previous"`
	PreviousOffset int             `json:"previous_offset"`
	HasMore        bool            `json:"has_more"`
	NextOffset     int             `json:"next_offset"`
}
