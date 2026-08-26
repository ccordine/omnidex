package worker

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func phpServiceHTTPLifecycleSource(plan phpServiceHTTPLifecyclePlan) (string, error) {
	var source strings.Builder
	source.WriteString(phpServiceHTTPLifecycleBlockerSource(plan.Blockers))
	for index, lifecycle := range plan.Lifecycles {
		if err := phpServiceWriteHTTPLifecyclePhase(&source, index+1, 1, lifecycle); err != nil {
			return "", err
		}
		if err := phpServiceWriteHTTPLifecyclePhase(&source, index+1, 2, lifecycle); err != nil {
			return "", err
		}
	}
	return source.String(), nil
}

func phpServiceWriteHTTPLifecyclePhase(
	source *strings.Builder,
	sequence, phase int,
	lifecycle phpServiceHTTPLifecycle,
) error {
	variable := fmt.Sprintf("$lifecycle%03dPhase%d", sequence, phase)
	shape, sentinels, err := phpServiceHTTPLifecycleSentinelShape(
		lifecycle.Interface, phase,
	)
	if err != nil {
		return err
	}
	source.WriteString(fmt.Sprintf(
		"%s = verificationLifecycleRequest(%s, %s, %s, %s);\n",
		variable, phpSingleQuoted(string(lifecycle.Writer.Endpoint.RequestMedia)),
		phpSingleQuoted(string(lifecycle.Writer.Endpoint.ResponseMedia)), shape, sentinels,
	))
	source.WriteString(fmt.Sprintf(
		"verifyHttpResponse(performHttpRequest(%s, %s, %s['headers'], %s['body']), %d, %s);\n",
		phpSingleQuoted(string(lifecycle.Writer.Endpoint.Method)),
		phpSingleQuoted(lifecycle.Writer.Fixture.Path), variable, variable,
		lifecycle.Writer.Endpoint.SuccessStatus,
		phpSingleQuoted(string(lifecycle.Writer.Endpoint.ResponseMedia)),
	))
	readerHeaders := phpServiceFixtureArray([]phpServiceFixturePair{{
		Key: "accept", Value: string(lifecycle.Reader.Endpoint.ResponseMedia),
	}})
	source.WriteString(fmt.Sprintf(
		"%sObserved = performHttpRequest('GET', %s, %s, '');\n",
		variable, phpSingleQuoted(lifecycle.Reader.Fixture.Path), readerHeaders,
	))
	source.WriteString(fmt.Sprintf(
		"verifyHttpResponse(%sObserved, %d, %s);\n",
		variable, lifecycle.Reader.Endpoint.SuccessStatus,
		phpSingleQuoted(string(lifecycle.Reader.Endpoint.ResponseMedia)),
	))
	source.WriteString(fmt.Sprintf(
		"verifyLifecycleSentinel(%sObserved, %s, %s['sentinels']);\n",
		variable, phpSingleQuoted(string(lifecycle.Reader.Endpoint.ResponseMedia)), variable,
	))
	return nil
}

func phpServiceHTTPLifecycleSentinelShape(
	stateInterface directCodingServiceStateInterfaceBinding,
	phase int,
) (string, string, error) {
	fields := make([]string, 0, len(stateInterface.Result.Fields))
	sentinels := make([]string, 0, len(stateInterface.Result.Fields))
	for index, field := range stateInterface.Result.Fields {
		value, leaves, err := phpServiceHTTPStateFieldSentinel(
			stateInterface.ID, field, phase, index+1,
		)
		if err != nil {
			return "", "", err
		}
		fields = append(fields, phpSingleQuoted(field.Name)+" => "+value)
		sentinels = append(sentinels, leaves...)
	}
	return "[" + strings.Join(fields, ", ") + "]",
		"[" + strings.Join(sentinels, ", ") + "]", nil
}

func phpServiceHTTPStateFieldSentinel(
	interfaceID string,
	field assemblyline.ApplicationServiceStateField,
	phase, ordinal int,
) (string, []string, error) {
	identity := fmt.Sprintf("%s/%d/%d/%s", interfaceID, phase, ordinal, field.Name)
	if field.Kind != assemblyline.ApplicationServiceStateRecordList {
		return phpServiceHTTPScalarSentinel(identity, field.Kind, phase)
	}
	record := make([]string, 0, len(field.RecordFields))
	leaves := make([]string, 0, len(field.RecordFields))
	for index, nested := range field.RecordFields {
		value, nestedLeaves, err := phpServiceHTTPScalarSentinel(
			fmt.Sprintf("%s/%d/%s", identity, index+1, nested.Name), nested.Kind, phase,
		)
		if err != nil {
			return "", nil, err
		}
		record = append(record, phpSingleQuoted(nested.Name)+" => "+value)
		leaves = append(leaves, nestedLeaves...)
	}
	return "[[" + strings.Join(record, ", ") + "]]", leaves, nil
}

func phpServiceHTTPScalarSentinel(
	identity string,
	kind assemblyline.ApplicationServiceStateFieldKind,
	phase int,
) (string, []string, error) {
	digest := sha256.Sum256([]byte(identity))
	integer := int64(binary.BigEndian.Uint32(digest[:4])%800000000) + 100000000
	textValue := fmt.Sprintf("odx-%d-%x", phase, digest[4:16])
	textSource := phpSingleQuoted(textValue)
	integerSource := fmt.Sprintf("%d", integer)
	numberSource := fmt.Sprintf("%d.375", integer)
	switch kind {
	case assemblyline.ApplicationServiceStateString:
		return textSource, []string{textSource}, nil
	case assemblyline.ApplicationServiceStateInteger:
		return integerSource, []string{integerSource}, nil
	case assemblyline.ApplicationServiceStateNumber:
		return numberSource, []string{numberSource}, nil
	case assemblyline.ApplicationServiceStateBoolean:
		if phase%2 == 0 {
			return "false", nil, nil
		}
		return "true", nil, nil
	case assemblyline.ApplicationServiceStateStringList:
		return "[" + textSource + "]", []string{textSource}, nil
	case assemblyline.ApplicationServiceStateIntegerList:
		return "[" + integerSource + "]", []string{integerSource}, nil
	case assemblyline.ApplicationServiceStateNumberList:
		return "[" + numberSource + "]", []string{numberSource}, nil
	case assemblyline.ApplicationServiceStateBooleanList:
		return "[true, false]", nil, nil
	default:
		return "", nil, fmt.Errorf(
			"HTTP lifecycle state interface has unsupported sentinel kind %q", kind,
		)
	}
}
