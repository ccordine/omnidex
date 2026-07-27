package main

import (
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
	"time"
)

func defaultProjectScopedSessionID(cwd string) string {
	clean := strings.TrimSpace(filepath.Clean(cwd))
	if clean == "" || clean == "." {
		return fmt.Sprintf("chat-%d", time.Now().Unix())
	}

	base := strings.ToLower(strings.TrimSpace(filepath.Base(clean)))
	base = normalizeSessionSlug(base)
	if base == "" {
		base = "workspace"
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.ToLower(clean)))
	return fmt.Sprintf("chat-%s-%08x", base, hasher.Sum32())
}

func normalizeSessionSlug(value string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if prevDash {
			continue
		}
		b.WriteRune('-')
		prevDash = true
	}
	return strings.Trim(b.String(), "-")
}
