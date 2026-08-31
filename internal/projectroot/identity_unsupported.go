//go:build !linux

package projectroot

import (
	"fmt"
	"runtime"
)

func DirectoryIdentity(_ string) (string, error) {
	return "", fmt.Errorf(
		"direct host-directory identity attestation is unsupported on %s",
		runtime.GOOS,
	)
}
