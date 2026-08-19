package queue

import (
	"fmt"
	"strings"
)

// transactionalBody returns exact migration semantics for execution inside a
// caller-owned transaction. Only one exact outer wrapper may be removed.
func (entry migrationBundleEntry) transactionalBody() ([]byte, error) {
	body := string(entry.body)
	const begin = "BEGIN;\n"
	const commit = "COMMIT;\n"
	if strings.HasPrefix(body, begin) && strings.HasSuffix(body, commit) {
		body = strings.TrimSuffix(strings.TrimPrefix(body, begin), commit)
	}
	upper := strings.ToUpper(body)
	if strings.Contains(upper, "BEGIN;") || strings.Contains(upper, "COMMIT;") ||
		strings.Contains(upper, "ROLLBACK;") {
		return nil, fmt.Errorf("migration %q has unsupported nested transaction control", entry.name)
	}
	return []byte(body), nil
}

func validateMigrationTransactionControl(entries []migrationBundleEntry) error {
	for index, entry := range entries {
		if _, err := entry.transactionalBody(); err != nil {
			return fmt.Errorf("migration bundle entry %d transaction control: %w", index, err)
		}
	}
	return nil
}
