package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
)

const browseUIPageSize = 25

func browsePageOptions(r *http.Request, fallbackLimit int, directoriesOnly bool) (hostbridge.BrowseOptions, error) {
	limit, err := exactChannelQueryInteger(r, "limit", fallbackLimit, 1, hostbridge.MaxBrowsePageSize)
	if err != nil {
		return hostbridge.BrowseOptions{}, err
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		return hostbridge.BrowseOptions{}, err
	}
	return hostbridge.BrowseOptions{
		Limit: limit, Offset: offset, DirectoriesOnly: directoriesOnly,
	}, nil
}

func (s *Server) projectAuthorizedBrowseOptions(
	ctx context.Context,
	target string,
	opts hostbridge.BrowseOptions,
) (hostbridge.BrowseOptions, error) {
	target = strings.TrimSpace(target)
	if target == "" || s.repo == nil {
		return opts, nil
	}
	root, found, err := s.repo.FindProjectBrowseRoot(ctx, target)
	if err != nil {
		return hostbridge.BrowseOptions{}, err
	}
	if found {
		opts.ExtraRoots = []string{root}
	}
	return opts, nil
}

func browsePreviousOffset(result hostbridge.BrowseResult) (int, bool, error) {
	if result.Offset == 0 {
		return 0, false, nil
	}
	if result.Limit < 1 || result.Limit > hostbridge.MaxBrowsePageSize {
		return 0, false, fmt.Errorf("directory page returned an invalid limit")
	}
	previous := result.Offset - result.Limit
	if previous < 0 {
		previous = 0
	}
	return previous, true, nil
}
