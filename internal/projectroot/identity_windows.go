package projectroot

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func directoryIdentityForHandle(directory *os.File) (string, error) {
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(directory.Fd()), &information); err != nil {
		return "", fmt.Errorf("read retained directory identity: %w", err)
	}
	fileID := uint64(information.FileIndexHigh)<<32 | uint64(information.FileIndexLow)
	identity := fmt.Sprintf("%s%d_%d", directoryIdentityPrefix, information.VolumeSerialNumber, fileID)
	if err := ValidateDirectoryIdentity(identity); err != nil {
		return "", err
	}
	return identity, nil
}
