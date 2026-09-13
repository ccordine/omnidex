//go:build !linux && !darwin && !windows

package projectroot

import (
	"fmt"
	"os"
	"runtime"
)

func directoryIdentityForHandle(_ *os.File) (string, error) {
	return "", fmt.Errorf(
		"directory identity attestation is unsupported on %s",
		runtime.GOOS,
	)
}
