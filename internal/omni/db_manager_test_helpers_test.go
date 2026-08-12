package omni

import "strings"

func normalizeSQLForTest(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}
