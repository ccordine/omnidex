package queue

import "fmt"

func workspaceMutationSQLState(
	present bool,
	sha string,
	size int64,
	mode uint32,
) (any, any, any, any) {
	if !present {
		return nil, nil, nil, nil
	}
	return "regular", sha, size, int32(mode)
}

func assignWorkspaceMutationSQLState(
	fileID, label string,
	present bool,
	kind, sha *string,
	size *int64,
	mode *int32,
) (string, int64, uint32, error) {
	if !present {
		if kind != nil || sha != nil || size != nil || mode != nil {
			return "", 0, 0, fmt.Errorf(
				"durable workspace mutation file %q has authority for absent %s state",
				fileID, label,
			)
		}
		return "", 0, 0, nil
	}
	if kind == nil || *kind != "regular" || sha == nil || size == nil || mode == nil || *mode <= 0 {
		return "", 0, 0, fmt.Errorf(
			"durable workspace mutation file %q has incomplete present %s state",
			fileID, label,
		)
	}
	return *sha, *size, uint32(*mode), nil
}
