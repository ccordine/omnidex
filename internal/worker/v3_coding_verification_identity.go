package worker

import "strings"

func validRepositoryVerificationOpaqueID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) &&
		validRepositoryVerificationSHA256(strings.TrimPrefix(value, prefix))
}

func validRepositoryVerificationSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			if char < 'a' || char > 'f' {
				return false
			}
		}
	}
	return true
}
