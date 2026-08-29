package queue

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

// FindProjectBrowseRoot returns one stored project location that is either an
// ancestor or descendant of the requested browse target. It deliberately
// selects only the location needed to authorize this target.
func (r *Repository) FindProjectBrowseRoot(ctx context.Context, target string) (string, bool, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false, fmt.Errorf("project browse target is required")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", false, fmt.Errorf("resolve project browse target: %w", err)
	}
	target = filepath.Clean(abs)

	var location string
	err = r.pool.QueryRow(ctx, `
		SELECT location
		FROM projects
		WHERE location = $1
		   OR location = $2
		   OR $1 = $2
		   OR (location <> $2 AND substring($1 FROM 1 FOR char_length(location) + 1) = location || $2)
		   OR ($1 <> $2 AND substring(location FROM 1 FOR char_length($1) + 1) = $1 || $2)
		ORDER BY char_length(location) DESC, id DESC
		LIMIT 1
	`, target, string(filepath.Separator)).Scan(&location)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find project browse root: %w", err)
	}
	location = filepath.Clean(strings.TrimSpace(location))
	if location == "" {
		return "", false, fmt.Errorf("project browse root query returned an empty location")
	}
	return location, true, nil
}
