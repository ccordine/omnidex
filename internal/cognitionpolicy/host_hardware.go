package cognitionpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const HostHardwareAttestationSchemaV2 = "omnidex.host-hardware-attestation.v2"

type HostHardwareAttestation struct {
	Schema                    string `json:"schema"`
	OS                        string `json:"os"`
	Architecture              string `json:"architecture"`
	LogicalCPUs               int    `json:"logical_cpus"`
	CPUIdentitySHA256         string `json:"cpu_identity_sha256"`
	AcceleratorIdentitySHA256 string `json:"accelerator_identity_sha256"`
	CPUEvidence               string `json:"cpu_evidence"`
	AcceleratorEvidence       string `json:"accelerator_evidence"`
	AttestationSHA256         string `json:"attestation_sha256"`
}

type hostHardwareProbe struct {
	goos, architecture string
	logicalCPUs        int
	readFile           func(string) ([]byte, error)
	glob               func(string) ([]string, error)
}

func AttestLocalHostHardware() (HostHardwareAttestation, error) {
	return attestHostHardware(hostHardwareProbe{
		goos: runtime.GOOS, architecture: runtime.GOARCH, logicalCPUs: runtime.NumCPU(),
		readFile: os.ReadFile, glob: filepath.Glob,
	})
}

// NewHostHardwareAttestation restores or constructs the exact bounded host
// identity evidence produced by a code-owned probe. Raw host labels are never
// accepted by this boundary.
func NewHostHardwareAttestation(
	operatingSystem string,
	architecture string,
	logicalCPUs int,
	cpuIdentitySHA256 string,
	acceleratorIdentitySHA256 string,
) (HostHardwareAttestation, error) {
	attestation := HostHardwareAttestation{
		Schema: HostHardwareAttestationSchemaV2,
		OS:     operatingSystem, Architecture: architecture, LogicalCPUs: logicalCPUs,
		CPUIdentitySHA256:         cpuIdentitySHA256,
		AcceleratorIdentitySHA256: acceleratorIdentitySHA256,
		CPUEvidence:               "linux:/proc/cpuinfo:selected-identity",
		AcceleratorEvidence:       "linux:/sys/class/drm/card*/device/vendor+device",
	}
	attestation.AttestationSHA256 = hostAttestationSHA256(attestation)
	if err := attestation.Validate(); err != nil {
		return HostHardwareAttestation{}, err
	}
	return attestation, nil
}

func attestHostHardware(probe hostHardwareProbe) (HostHardwareAttestation, error) {
	if probe.goos != "linux" || probe.architecture == "" || probe.logicalCPUs < 1 ||
		probe.readFile == nil || probe.glob == nil {
		return HostHardwareAttestation{}, fmt.Errorf("host hardware attestation requires an exact Linux probe")
	}
	cpuRaw, err := probe.readFile("/proc/cpuinfo")
	if err != nil {
		return HostHardwareAttestation{}, fmt.Errorf("read code-owned CPU identity: %w", err)
	}
	cpuIdentity, err := selectedCPUIdentity(string(cpuRaw))
	if err != nil {
		return HostHardwareAttestation{}, err
	}
	accelerators, err := selectedAcceleratorIdentity(probe)
	if err != nil {
		return HostHardwareAttestation{}, err
	}
	return NewHostHardwareAttestation(
		probe.goos, probe.architecture, probe.logicalCPUs,
		exactStringListSHA256(cpuIdentity), exactStringListSHA256(accelerators),
	)
}

func (attestation HostHardwareAttestation) Validate() error {
	if attestation.Schema != HostHardwareAttestationSchemaV2 ||
		attestation.OS != "linux" || !validExactName(attestation.Architecture, 64) ||
		attestation.LogicalCPUs < 1 ||
		!validPolicySHA256(attestation.CPUIdentitySHA256) ||
		!validPolicySHA256(attestation.AcceleratorIdentitySHA256) ||
		attestation.CPUEvidence != "linux:/proc/cpuinfo:selected-identity" ||
		attestation.AcceleratorEvidence != "linux:/sys/class/drm/card*/device/vendor+device" ||
		!validPolicySHA256(attestation.AttestationSHA256) ||
		attestation.AttestationSHA256 != hostAttestationSHA256(attestation) {
		return fmt.Errorf("%w: host hardware attestation is invalid", ErrInvalidBrain)
	}
	return nil
}

func selectedCPUIdentity(raw string) ([]string, error) {
	allowed := map[string]struct{}{
		"vendor_id": {}, "model name": {}, "hardware": {},
		"cpu implementer": {}, "cpu part": {},
	}
	seen := make(map[string]struct{})
	for _, line := range strings.Split(raw, "\n") {
		key, value, exists := strings.Cut(line, ":")
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		if !exists || value == "" {
			continue
		}
		if _, accepted := allowed[key]; !accepted {
			continue
		}
		seen[key+"="+value] = struct{}{}
	}
	identity := make([]string, 0, len(seen))
	for value := range seen {
		identity = append(identity, value)
	}
	sort.Strings(identity)
	if len(identity) == 0 {
		return nil, fmt.Errorf("code-owned CPU identity contains no registered fields")
	}
	return identity, nil
}

func selectedAcceleratorIdentity(probe hostHardwareProbe) ([]string, error) {
	paths, err := probe.glob("/sys/class/drm/card*/device/vendor")
	if err != nil {
		return nil, fmt.Errorf("enumerate code-owned accelerator identity: %w", err)
	}
	identities := make([]string, 0, len(paths))
	for _, vendorPath := range paths {
		vendor, err := probe.readFile(vendorPath)
		if err != nil {
			return nil, fmt.Errorf("read accelerator vendor identity: %w", err)
		}
		device, err := probe.readFile(filepath.Join(filepath.Dir(vendorPath), "device"))
		if err != nil {
			return nil, fmt.Errorf("read accelerator device identity: %w", err)
		}
		identities = append(identities,
			strings.TrimSpace(string(vendor))+":"+strings.TrimSpace(string(device)))
	}
	if len(identities) == 0 {
		identities = append(identities, "none-visible")
	}
	sort.Strings(identities)
	return identities, nil
}

func exactStringListSHA256(values []string) string {
	raw, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func hostAttestationSHA256(attestation HostHardwareAttestation) string {
	copy := attestation
	copy.AttestationSHA256 = ""
	raw, err := canonicalPolicyJSON(copy)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
