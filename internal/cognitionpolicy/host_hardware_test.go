package cognitionpolicy

import (
	"fmt"
	"strings"
	"testing"
)

func TestHostHardwareAttestationUsesStableCodeOwnedDigests(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"/proc/cpuinfo":                      "vendor_id : AuthenticAMD\nmodel name : Example CPU\nprocessor : 0\n",
		"/sys/class/drm/card0/device/vendor": "0x1002\n",
		"/sys/class/drm/card0/device/device": "0x15bf\n",
	}
	probe := hostHardwareProbe{
		goos: "linux", architecture: "amd64", logicalCPUs: 16,
		readFile: func(path string) ([]byte, error) {
			value, exists := files[path]
			if !exists {
				return nil, fmt.Errorf("unexpected path %s", path)
			}
			return []byte(value), nil
		},
		glob: func(string) ([]string, error) {
			return []string{"/sys/class/drm/card0/device/vendor"}, nil
		},
	}
	first, err := attestHostHardware(probe)
	if err != nil {
		t.Fatal(err)
	}
	second, err := attestHostHardware(probe)
	if err != nil || second != first {
		t.Fatalf("repeat attestation=%+v error=%v", second, err)
	}
	if strings.Contains(first.CPUIdentitySHA256, "Example CPU") ||
		first.AttestationSHA256 == "" {
		t.Fatalf("hardware evidence was not reduced to a stable digest: %+v", first)
	}
	changed := first
	changed.LogicalCPUs++
	if err := changed.Validate(); err == nil {
		t.Fatal("changed host hardware retained a valid attestation")
	}
}
