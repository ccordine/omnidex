package queue

import "fmt"

func repositoryMutationSQLState(
	present bool,
	sha string,
	size int64,
	mode uint32,
) (any, any, any) {
	if !present {
		return nil, nil, nil
	}
	return sha, size, int32(mode)
}

func assignRepositoryMutationSQLState(
	fileID, name string,
	present bool,
	sha *string,
	size *int64,
	mode *int32,
) (string, int64, uint32, error) {
	if !present {
		if sha != nil || size != nil || mode != nil {
			return "", 0, 0, fmt.Errorf(
				"durable repository mutation file %q has nonempty absent %s state", fileID, name,
			)
		}
		return "", 0, 0, nil
	}
	if sha == nil || size == nil || mode == nil || *mode < 0 {
		return "", 0, 0, fmt.Errorf(
			"durable repository mutation file %q has incomplete present %s state", fileID, name,
		)
	}
	return *sha, *size, uint32(*mode), nil
}
