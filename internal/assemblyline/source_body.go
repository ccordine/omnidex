package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxSourceBodyAttempts                = 3
	MaxSourceBodyCorrectionQuestionBytes = 2048
	MaxSourceBodyResponseBytes           = 32 * 1024
	MaxSourceCapabilityContextBytes      = 32 * 1024
)

// NormalizeSourceBodyResponse validates only the ordinary text returned for a
// source-body job. Declaration syntax and every surrounding structural byte
// are supplied later by the code-owned source adapter.
func NormalizeSourceBodyResponse(raw string) (string, error) {
	if raw == "" || !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("source-body response must be non-empty UTF-8 without NUL bytes")
	}
	if len(raw) > MaxSourceBodyResponseBytes {
		return "", fmt.Errorf("source-body response exceeds %d bytes", MaxSourceBodyResponseBytes)
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return "", fmt.Errorf("source-body response is empty")
	}
	return normalized, nil
}

// ComposeSourceDeclaration applies one validated body inside the declaration
// shape already owned by code. The model never has to echo the signature,
// parameter list, return type, name, export modifier, or closing structure.
func ComposeSourceDeclaration(signature, rawBody string) (string, error) {
	signature = strings.TrimSpace(signature)
	if signature == "" || strings.ContainsAny(signature, "\x00\r\n") ||
		!utf8.ValidString(signature) {
		return "", fmt.Errorf("source declaration signature must be one trimmed line")
	}
	body, err := NormalizeSourceBodyResponse(rawBody)
	if err != nil {
		return "", err
	}
	return signature + " {\n" + body + "\n}", nil
}

// SourceBodyDefect is a validator-owned proof that one exact byte span, and no
// surrounding source, is unresolved. Ordinary validation errors are not
// correctable by inference.
type SourceBodyDefect struct {
	message               string
	question              string
	source                string
	startByte             int
	endByte               int
	identifierReplacement bool
	replacements          []OpaqueModelChoice
}

func NewSourceBodyDefect(
	source string,
	startByte int,
	endByte int,
	question string,
	validationErr error,
) (*SourceBodyDefect, error) {
	if validationErr == nil {
		return nil, fmt.Errorf("source-body defect requires one deterministic validation failure")
	}
	if err := validateSourceBodySpan(source, startByte, endByte); err != nil {
		return nil, err
	}
	question = strings.TrimSpace(question)
	if question == "" || !utf8.ValidString(question) || strings.ContainsRune(question, '\x00') ||
		len(question) > MaxSourceBodyCorrectionQuestionBytes {
		return nil, fmt.Errorf("source-body defect question must be bounded non-empty UTF-8 text")
	}
	message := strings.TrimSpace(validationErr.Error())
	if message == "" || !utf8.ValidString(message) || strings.ContainsRune(message, '\x00') {
		return nil, fmt.Errorf("source-body defect requires one valid deterministic diagnostic")
	}
	return &SourceBodyDefect{
		message: message, question: question, source: source,
		startByte: startByte, endByte: endByte,
	}, nil
}

// NewSourceBodyIdentifierDefect records that the exact failed span is one
// identifier whose replacement is already enumerated by deterministic scope
// analysis. The replacement values stay code-owned. A sole value is applied
// without inference; multiple values are projected as opaque choices.
func NewSourceBodyIdentifierDefect(
	source string,
	startByte int,
	endByte int,
	question string,
	validationErr error,
	replacements []OpaqueModelChoice,
) (*SourceBodyDefect, error) {
	defect, err := NewSourceBodyDefect(
		source, startByte, endByte, question, validationErr,
	)
	if err != nil {
		return nil, err
	}
	if len(replacements) > 0 {
		if err := validateOpaqueModelChoices(replacements); err != nil {
			return nil, fmt.Errorf("source-body identifier replacements: %w", err)
		}
		defect.replacements = append([]OpaqueModelChoice(nil), replacements...)
	}
	defect.identifierReplacement = true
	return defect, nil
}

// WithIdentifierReplacements binds scope-derived choices to a located
// identifier defect without changing its source or byte range.
func (defect *SourceBodyDefect) WithIdentifierReplacements(
	replacements []OpaqueModelChoice,
) (*SourceBodyDefect, error) {
	if defect == nil {
		return nil, fmt.Errorf("source-body defect is nil")
	}
	if err := validateOpaqueModelChoices(replacements); err != nil {
		return nil, fmt.Errorf("source-body identifier replacements: %w", err)
	}
	bound := *defect
	bound.identifierReplacement = true
	bound.replacements = append([]OpaqueModelChoice(nil), replacements...)
	return &bound, nil
}

func (defect *SourceBodyDefect) Error() string {
	if defect == nil {
		return "source-body defect is nil"
	}
	return defect.message
}

func (defect *SourceBodyDefect) RequiresIdentifierReplacement() bool {
	return defect != nil && defect.identifierReplacement
}

func (defect *SourceBodyDefect) Mutable(source string) (string, error) {
	startByte, endByte, err := defect.MutableRange(source)
	if err != nil {
		return "", err
	}
	return source[startByte:endByte], nil
}

func (defect *SourceBodyDefect) MutableRange(source string) (int, int, error) {
	if defect == nil || defect.source == "" ||
		defect.source != source {
		return 0, 0, fmt.Errorf(
			"source-body defect does not bind to the current code-owned source",
		)
	}
	if err := validateSourceBodySpan(source, defect.startByte, defect.endByte); err != nil {
		return 0, 0, err
	}
	return defect.startByte, defect.endByte, nil
}

// Correction binds the validator proof to the exact code-owned source state.
// The returned value retains the complete state for deterministic splicing,
// but its model projection exposes only the proven mutable bytes and question.
func (defect *SourceBodyDefect) Correction(source string) (SourceBodyCorrection, error) {
	if defect == nil || defect.source == "" ||
		defect.source != source {
		return SourceBodyCorrection{}, fmt.Errorf(
			"source-body defect does not bind to the current code-owned source",
		)
	}
	if err := validateSourceBodySpan(source, defect.startByte, defect.endByte); err != nil {
		return SourceBodyCorrection{}, err
	}
	correction := SourceBodyCorrection{
		base: source, question: defect.question,
		startByte: defect.startByte, endByte: defect.endByte,
		identifierReplacement: defect.identifierReplacement,
		replacements:          append([]OpaqueModelChoice(nil), defect.replacements...),
	}
	if err := correction.Validate(); err != nil {
		return SourceBodyCorrection{}, err
	}
	return correction, nil
}
