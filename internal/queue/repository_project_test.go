package queue

import "testing"

func TestProjectLocationFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		want     string
	}{
		{
			name:     "uses client cwd",
			metadata: `{"client_cwd":"/home/gryph/Projects/ai/omnidex"}`,
			want:     "/home/gryph/Projects/ai/omnidex",
		},
		{
			name:     "falls back to host cwd",
			metadata: `{"host_env_cwd":"/tmp/work"}`,
			want:     "/tmp/work",
		},
		{
			name:     "ignores non-string values",
			metadata: `{"client_cwd":123,"host_env_cwd":false}`,
			want:     "",
		},
		{
			name:     "invalid json",
			metadata: `{`,
			want:     "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := projectLocationFromMetadata([]byte(tc.metadata))
			if got != tc.want {
				t.Fatalf("projectLocationFromMetadata(%q)=%q want %q", tc.metadata, got, tc.want)
			}
		})
	}
}

func TestProjectNameFromLocation(t *testing.T) {
	tests := []struct {
		location string
		want     string
	}{
		{location: "/home/gryph/Projects/ai/omnidex", want: "omnidex"},
		{location: "/tmp/workspace/", want: "workspace"},
		{location: ".", want: "workspace"},
		{location: "", want: "workspace"},
	}

	for _, tc := range tests {
		got := projectNameFromLocation(tc.location)
		if got != tc.want {
			t.Fatalf("projectNameFromLocation(%q)=%q want %q", tc.location, got, tc.want)
		}
	}
}
