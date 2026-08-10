//go:build unix

package cognitiongauntlet

import (
	"fmt"
	"os"
	"syscall"
)

func validatePrivateOutputDirectory(path string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("offline private output directory %q lacks exclusive owner authority", path)
	}
	return nil
}
