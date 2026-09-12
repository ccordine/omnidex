//go:build linux

package projectroot

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDirectoryIdentityUsesActualDeviceAndInode(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"source", "reports"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), name)
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			identity, err := DirectoryIdentity(root)
			if err != nil {
				t.Fatal(err)
			}
			var status unix.Stat_t
			if err := unix.Stat(root, &status); err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("directory_%d_%d", status.Dev, status.Ino); identity != want {
				t.Fatalf("identity=%q, want actual stat %q", identity, want)
			}
			if err := os.Rename(root, root+"-previous"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			replacement, err := DirectoryIdentity(root)
			if err != nil {
				t.Fatal(err)
			}
			if replacement == identity {
				t.Fatal("a replacement directory reused the retained directory identity")
			}
		})
	}
}

func TestDirectoryIdentityRejectsInvalidStatValues(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"", "directory_identity_v1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"directory_1_0", "directory_01_2", "directory_1_02", "directory_-1_2",
		"directory_1_2_3", "directory_18446744073709551616_1",
	} {
		if err := ValidateDirectoryIdentity(value); err == nil {
			t.Fatalf("accepted invalid directory stat %q", value)
		}
	}
}
