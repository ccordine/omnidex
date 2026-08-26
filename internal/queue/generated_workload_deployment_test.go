package queue

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func generatedDeploymentTestCommand() GeneratedWorkloadDeploymentCommand {
	return GeneratedWorkloadDeploymentCommand{
		Authority: GeneratedWorkloadDeploymentAuthority{
			JobID: 11, Generation: 2, StepID: 33, ProjectID: 5,
		},
		DeploymentIntentJobID:          strings.Repeat("1", 64),
		DeploymentIntentResponseSHA256: strings.Repeat("2", 64),
		Disposition:                    GeneratedWorkloadDeploymentPersistCurrentHost,
		WorkspaceSHA256:                strings.Repeat("3", 64),
		SourceSnapshotSHA256:           strings.Repeat("4", 64),
		AdapterID:                      "container.compose",
		AdapterVersion:                 "1.0.0",
		ProfileID:                      "http.service",
		ProfileVersion:                 "2",
		ComposeFileID:                  "file_" + strings.Repeat("5", 64),
		ComposeFileSHA256:              strings.Repeat("6", 64),
		ComposeProject:                 "generated-11-g2",
		ConfigSHA256:                   strings.Repeat("7", 64),
		BindHost:                       GeneratedWorkloadDeploymentBindLoopback,
		EndpointPortAuthority:          GeneratedWorkloadDeploymentPortAllocate,
		EndpointScheme:                 "https",
		EndpointHost:                   "service.example.test",
		EndpointPort:                   0,
		EndpointPath:                   "/ready",
		Services:                       []string{"api", "gateway"},
		RequiredSecretNames:            []string{"APPLICATION_KEY", "DATABASE_PASSWORD"},
		SecretSetSHA256:                strings.Repeat("8", 64),
	}
}

func generatedDeploymentTestReceipt(
	t *testing.T,
	command GeneratedWorkloadDeploymentCommand,
) GeneratedWorkloadDeploymentReceipt {
	t.Helper()
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	appliedAt := time.Unix(1_800_000_000, 123_456_000).UTC()
	services := make([]GeneratedWorkloadDeploymentServiceReceipt, len(command.Services))
	for index, name := range command.Services {
		services[index] = GeneratedWorkloadDeploymentServiceReceipt{
			Service: name, ContainerID: strings.Repeat(string(rune('a'+index)), 64),
			ImageDigest:   "sha256:" + strings.Repeat(string(rune('c'+index)), 64),
			RestartPolicy: GeneratedWorkloadDeploymentRestartUnlessStopped,
			State:         "running", Health: "healthy",
		}
	}
	return GeneratedWorkloadDeploymentReceipt{
		Schema: GeneratedWorkloadDeploymentReceiptV2, OperationID: identity.OperationID,
		ConfigSHA256: command.ConfigSHA256, ComposeProject: command.ComposeProject,
		EndpointScheme: command.EndpointScheme, EndpointHost: command.EndpointHost,
		EndpointPort: 18443, EndpointPath: command.EndpointPath,
		Services: services, AppliedAt: appliedAt, ObservedAt: appliedAt.Add(time.Second),
		WorkspaceVerificationReceiptID: "generated_workload_verification_" + strings.Repeat("9", 64),
		ExecutionEvidenceIDs:           []int64{41, 42, 43, 44, 45, 46},
		ObservationEvidenceIDs:         []int64{51, 52},
		PriorDeploymentID:              command.PriorDeploymentID,
	}
}

func TestGeneratedWorkloadDeploymentIdentityBindsExactContent(t *testing.T) {
	command := generatedDeploymentTestCommand()
	first, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.HasPrefix(first.OperationID, "generated_workload_deployment_") {
		t.Fatalf("generated deployment identity is not deterministic: %#v %#v", first, second)
	}
	command.SourceSnapshotSHA256 = strings.Repeat("8", 64)
	changed, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	if changed.OperationID == first.OperationID {
		t.Fatal("source snapshot change retained deployment operation identity")
	}
	command = generatedDeploymentTestCommand()
	command.SecretSetSHA256 = strings.Repeat("9", 64)
	changed, err = generatedWorkloadDeploymentOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	if changed.OperationID == first.OperationID {
		t.Fatal("secret-set rotation retained deployment operation identity")
	}
}

func TestGeneratedWorkloadDeploymentRejectsSecretValuesAndUnsafeEndpoint(t *testing.T) {
	for name, mutate := range map[string]func(*GeneratedWorkloadDeploymentCommand){
		"secret value": func(command *GeneratedWorkloadDeploymentCommand) {
			command.RequiredSecretNames = []string{"PASSWORD=raw-secret"}
		},
		"query": func(command *GeneratedWorkloadDeploymentCommand) {
			command.EndpointPath = "/ready?token=raw-secret"
		},
		"uppercase host": func(command *GeneratedWorkloadDeploymentCommand) {
			command.EndpointHost = "Service.example.test"
		},
		"empty DNS label": func(command *GeneratedWorkloadDeploymentCommand) {
			command.EndpointHost = "service..example.test"
		},
		"unclean path": func(command *GeneratedWorkloadDeploymentCommand) {
			command.EndpointPath = "/health/../ready"
		},
		"unsorted services": func(command *GeneratedWorkloadDeploymentCommand) {
			command.Services = []string{"gateway", "api"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			command := generatedDeploymentTestCommand()
			mutate(&command)
			if err := validateGeneratedWorkloadDeploymentCommand(command); err == nil {
				t.Fatalf("invalid command was accepted: %+v", command)
			}
		})
	}
}

func TestGeneratedWorkloadDeploymentPortAllocationIsExplicit(t *testing.T) {
	allocated := generatedDeploymentTestCommand()
	allocated.EndpointPort = 18443
	if err := validateGeneratedWorkloadDeploymentCommand(allocated); err == nil {
		t.Fatal("allocated port authority accepted a preselected port")
	}
	fixed := generatedDeploymentTestCommand()
	fixed.EndpointPortAuthority = GeneratedWorkloadDeploymentPortFixed
	if err := validateGeneratedWorkloadDeploymentCommand(fixed); err == nil {
		t.Fatal("fixed port authority accepted port zero")
	}
	fixed.EndpointPort = 18443
	identity, err := generatedWorkloadDeploymentOperation(fixed)
	if err != nil {
		t.Fatal(err)
	}
	receipt := generatedDeploymentTestReceipt(t, fixed)
	receipt.EndpointPort = 18444
	if err := validateGeneratedWorkloadDeploymentReceipt(fixed, receipt, identity); err == nil {
		t.Fatal("fixed port receipt changed immutable endpoint authority")
	}
}

func TestGeneratedWorkloadDeploymentReceiptRequiresHealthyExactServices(t *testing.T) {
	command := generatedDeploymentTestCommand()
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	receipt := generatedDeploymentTestReceipt(t, command)
	encoded, digest, err := canonicalGeneratedWorkloadDeploymentReceipt(command, receipt, identity)
	if err != nil {
		t.Fatal(err)
	}
	if digest != generatedDeploymentSHA(encoded) || !json.Valid([]byte(encoded)) {
		t.Fatalf("invalid canonical receipt digest %q for %q", digest, encoded)
	}
	oversized := generatedDeploymentTestReceipt(t, command)
	oversized.ExecutionEvidenceIDs = make([]int64, 10)
	for index := range oversized.ExecutionEvidenceIDs {
		oversized.ExecutionEvidenceIDs[index] = int64(index + 1)
	}
	if _, _, err := canonicalGeneratedWorkloadDeploymentReceipt(command, oversized, identity); err == nil {
		t.Fatal("oversized execution evidence set was accepted")
	}
	receipt.Services[0].Health = "starting"
	if _, _, err := canonicalGeneratedWorkloadDeploymentReceipt(command, receipt, identity); err == nil {
		t.Fatal("unhealthy service receipt was accepted")
	}
	receipt = generatedDeploymentTestReceipt(t, command)
	receipt.ExecutionEvidenceIDs = []int64{42, 41, 43, 44, 45, 46}
	if _, _, err := canonicalGeneratedWorkloadDeploymentReceipt(command, receipt, identity); err == nil {
		t.Fatal("unsorted execution evidence identities were accepted")
	}
}

func TestGeneratedWorkloadDeploymentTypesExposeNoRawSecretOrRuntimePayload(t *testing.T) {
	for _, value := range []any{
		GeneratedWorkloadDeploymentCommand{}, GeneratedWorkloadDeploymentReceipt{},
		GeneratedWorkloadDeploymentServiceReceipt{}, GeneratedWorkloadDeploymentTransition{},
	} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			field := strings.ToLower(typeOf.Field(index).Name + " " + typeOf.Field(index).Tag.Get("json"))
			for _, forbidden := range []string{"secretvalue", "environment", "rawconfig", "stdout", "stderr"} {
				if strings.Contains(field, forbidden) {
					t.Fatalf("%s contains forbidden payload field %q", typeOf.Name(), field)
				}
			}
		}
	}
}

func TestGeneratedWorkloadDeploymentTransitionGraphIsCodeOwned(t *testing.T) {
	if !generatedDeploymentTransitionAllowed(GeneratedWorkloadDeploymentPrepared, GeneratedWorkloadDeploymentApplying) ||
		generatedDeploymentTransitionAllowed(GeneratedWorkloadDeploymentApplied, GeneratedWorkloadDeploymentRolledBack) ||
		generatedDeploymentTransitionAllowed(GeneratedWorkloadDeploymentIndeterminate, GeneratedWorkloadDeploymentApplied) ||
		generatedDeploymentTransitionAllowed(GeneratedWorkloadDeploymentFailed, GeneratedWorkloadDeploymentApplying) ||
		generatedDeploymentTransitionAllowed(GeneratedWorkloadDeploymentPrepared, GeneratedWorkloadDeploymentApplied) {
		t.Fatal("generated deployment transition graph differs from registered authority")
	}
}
