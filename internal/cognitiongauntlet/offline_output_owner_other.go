//go:build !unix

package cognitiongauntlet

import (
	"fmt"
	"os"
)

func validatePrivateOutputDirectory(path string, _ os.FileInfo) error {
	return fmt.Errorf("offline private output ownership cannot be attested for %q on this platform", path)
}
