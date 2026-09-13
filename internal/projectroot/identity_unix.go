//go:build linux || darwin

package projectroot

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func directoryIdentityForHandle(directory *os.File) (string, error) {
	var status unix.Stat_t
	if err := unix.Fstat(int(directory.Fd()), &status); err != nil {
		return "", fmt.Errorf("stat retained directory identity: %w", err)
	}
	identity := fmt.Sprintf("%s%d_%d", directoryIdentityPrefix, uint64(status.Dev), status.Ino)
	if err := ValidateDirectoryIdentity(identity); err != nil {
		return "", err
	}
	return identity, nil
}
