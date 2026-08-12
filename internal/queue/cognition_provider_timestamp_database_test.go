package queue

import "testing"

func TestPostgresExactProviderTimestampMatchesGoAuthority(t *testing.T) {
	_, pool, ctx := policyInputFreshRepository(t)
	cases := []struct {
		name      string
		value     string
		precision int
		want      bool
	}{
		{"response_seconds", `"2026-08-09T22:00:00Z"`, 9, true},
		{"response_nanoseconds", `"2026-08-09T22:00:00.123456789Z"`, 9, true},
		{"observation_microseconds", `"2026-08-09T22:00:00.123456Z"`, 6, true},
		{"year_zero", `"0000-08-09T22:00:00Z"`, 9, false},
		{"hour_24", `"2026-08-09T24:00:00Z"`, 9, false},
		{"second_60", `"2026-08-09T22:00:60Z"`, 9, false},
		{"invalid_day", `"2026-02-30T22:00:00Z"`, 9, false},
		{"trailing_fraction_zero", `"2026-08-09T22:00:00.100Z"`, 9, false},
		{"observation_nanoseconds", `"2026-08-09T22:00:00.1234567Z"`, 6, false},
		{"not_string", `24`, 9, false},
		{"null", `null`, 9, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var got bool
			if err := pool.QueryRow(ctx,
				`SELECT cognition_provider_timestamp_is_exact($1::jsonb,$2)`,
				test.value, test.precision,
			).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("timestamp authority=%v, want %v", got, test.want)
			}
		})
	}
}
