package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/modelcontext"
)

const testingLibraryMissingAccessibleRolePrefix = "Unable to find an accessible element with the role \""
const testingLibraryMissingAvailableRolePrefix = "Unable to find an element with the role \""

func directCodingTestingLibraryRoleObservationProjection(
	errorName string,
	primaryMessage string,
	observation *directCodingTestingLibraryRoleObservation,
) (string, error) {
	if errorName != "TestingLibraryElementError" {
		if observation != nil {
			return "", fmt.Errorf("non-Testing-Library failure carried an accessibility observation")
		}
		return "", nil
	}
	role, visibility, recognized, err := directCodingTestingLibraryRequestedRole(primaryMessage)
	if err != nil {
		return "", err
	}
	if !recognized {
		if observation != nil {
			return "", fmt.Errorf("non-role Testing Library failure carried a role observation")
		}
		return "", nil
	}
	if observation == nil {
		return "", fmt.Errorf("Testing Library missing-role failure omitted its structured role observation")
	}
	if observation.Schema != directCodingTestingLibraryRoleObservationSchemaV1 {
		return "", fmt.Errorf("Testing Library role observation has unsupported schema %q", observation.Schema)
	}
	if observation.RequestedRole != role {
		return "", fmt.Errorf(
			"Testing Library role observation requested role %q does not match failure role %q",
			observation.RequestedRole, role,
		)
	}
	if observation.Visibility != visibility {
		return "", fmt.Errorf(
			"Testing Library role observation visibility %q does not match failure visibility %q",
			observation.Visibility, visibility,
		)
	}
	if observation.Status != directCodingTestingLibraryRoleObservationStatusComplete {
		return "", fmt.Errorf(
			"Testing Library role observation is incomplete with status %q",
			observation.Status,
		)
	}
	if observation.ElementCount < 0 || int64(len(observation.Names)) != observation.ElementCount {
		return "", fmt.Errorf("Testing Library complete role observation has contradictory element count")
	}

	switch observation.ElementCount {
	case 0:
		return "Runtime accessibility observation: the requested role currently has zero elements.", nil
	case 1:
		name, err := directCodingObservedAccessibleNameProjection(observation.Names[0])
		if err != nil {
			return "", err
		}
		return "Runtime accessibility observation: the requested role currently has one element with computed accessible name " + name + ".", nil
	default:
		names := make([]string, len(observation.Names))
		for index, raw := range observation.Names {
			projected, err := directCodingObservedAccessibleNameProjection(raw)
			if err != nil {
				return "", fmt.Errorf("project observed accessible name %d: %w", index, err)
			}
			names[index] = projected
		}
		return fmt.Sprintf(
			"Runtime accessibility observation: the requested role currently has %d elements with computed accessible names in DOM order [%s].",
			observation.ElementCount, strings.Join(names, "; "),
		), nil
	}
}

func directCodingTestingLibraryRequestedRole(
	message string,
) (string, directCodingTestingLibraryRoleVisibility, bool, error) {
	for _, candidate := range []struct {
		prefix     string
		visibility directCodingTestingLibraryRoleVisibility
	}{
		{testingLibraryMissingAccessibleRolePrefix, directCodingTestingLibraryRoleVisibilityAccessible},
		{testingLibraryMissingAvailableRolePrefix, directCodingTestingLibraryRoleVisibilityAvailable},
	} {
		if !strings.HasPrefix(message, candidate.prefix) {
			continue
		}
		remainder := strings.TrimPrefix(message, candidate.prefix)
		closing := strings.IndexByte(remainder, '"')
		if closing <= 0 {
			return "", "", false, fmt.Errorf("Testing Library missing-role failure has malformed requested role")
		}
		role := remainder[:closing]
		if len(role) > 64 || !directCodingTestingLibraryRoleToken(role) {
			return "", "", false, fmt.Errorf("Testing Library missing-role failure has unsupported requested role %q", role)
		}
		return role, candidate.visibility, true, nil
	}
	return "", "", false, nil
}

func directCodingTestingLibraryRoleToken(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] >= 'a' && value[index] <= 'z' ||
			index > 0 && value[index] >= '0' && value[index] <= '9' ||
			index > 0 && value[index] == '-' {
			continue
		}
		return false
	}
	return value != ""
}

func directCodingObservedAccessibleNameProjection(value string) (string, error) {
	direct := "exact text " + strconv.QuoteToGraphic(value)
	if !strings.ContainsAny(value, `/\\`) && !modelcontext.ContainsPathIdentity(direct) {
		return direct, nil
	}
	encoded := "exact UTF-8 byte sequence [" + directCodingAccessibleNameByteSequence(value) + "]"
	if modelcontext.ContainsPathIdentity(encoded) {
		return "", fmt.Errorf("observed accessible-name projection retained path identity")
	}
	return encoded, nil
}

func directCodingAccessibleNameByteSequence(value string) string {
	if value == "" {
		return "empty"
	}
	parts := make([]string, 0, len(value))
	for offset := 0; offset < len(value); {
		if directCodingRegularExpressionTextByte(value[offset]) {
			end := offset + 1
			for end < len(value) && directCodingRegularExpressionTextByte(value[end]) {
				end++
			}
			parts = append(parts, `text "`+value[offset:end]+`"`)
			offset = end
			continue
		}
		parts = append(parts, describeDirectCodingRegularExpressionByte(value[offset]))
		offset++
	}
	return strings.Join(parts, "; ")
}
