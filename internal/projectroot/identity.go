package projectroot

import (
	"fmt"
	"strconv"
	"strings"
)

const directoryIdentityPrefix = "directory_"

// ValidateDirectoryIdentity checks the shape of a device/inode pair. These
// values are local filesystem facts, not credentials or proof of execution.
func ValidateDirectoryIdentity(value string) error {
	if !strings.HasPrefix(value, directoryIdentityPrefix) {
		return fmt.Errorf("directory identity requires a device and inode")
	}
	parts := strings.Split(strings.TrimPrefix(value, directoryIdentityPrefix), "_")
	if len(parts) != 2 {
		return fmt.Errorf("directory identity requires exactly one device/inode pair")
	}
	for index, part := range parts {
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil || strconv.FormatUint(number, 10) != part || (index == 1 && number == 0) {
			return fmt.Errorf("directory identity contains an invalid device or inode")
		}
	}
	return nil
}
