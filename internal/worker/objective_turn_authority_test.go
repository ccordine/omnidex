package worker

import "encoding/json"

func objectiveAssistantMetadata() json.RawMessage {
	return json.RawMessage(`{"channel_id":"test-chat","channel_mode":"assistant"}`)
}

func objectiveAssistantDataSourceMetadata() json.RawMessage {
	return json.RawMessage(`{
		"channel_id":"test-chat",
		"channel_mode":"assistant",
		"data_source_id":"source-1"
	}`)
}
