package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/sourcebodyresponse"
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
	sourceSHA256          string
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
		message: message, question: question, sourceSHA256: sourceBodySHA256(source),
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
// identifier defect without changing its source digest or byte range.
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
	if defect == nil || defect.sourceSHA256 == "" ||
		defect.sourceSHA256 != sourceBodySHA256(source) {
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
	if defect == nil || defect.sourceSHA256 == "" ||
		defect.sourceSHA256 != sourceBodySHA256(source) {
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

// SourceBodyCorrection is code-owned mutation state. The provider returns
// ordinary text for the one mutable span.
type SourceBodyCorrection struct {
	base                  string
	question              string
	startByte             int
	endByte               int
	identifierReplacement bool
	replacements          []OpaqueModelChoice
}

type SourceBodyCorrectionEvidence struct {
	BaseCandidate  string
	BaseSHA256     string
	StartByte      int
	EndByte        int
	Question       string
	QuestionSHA256 string
}

func (evidence SourceBodyCorrectionEvidence) Validate(modelInput string) error {
	if err := validateSourceBodySpan(
		evidence.BaseCandidate, evidence.StartByte, evidence.EndByte,
	); err != nil {
		return err
	}
	if evidence.BaseSHA256 != sourceBodySHA256(evidence.BaseCandidate) ||
		evidence.QuestionSHA256 != sourceBodySHA256(evidence.Question) {
		return fmt.Errorf("source-body correction evidence digest does not match")
	}
	question := strings.TrimSpace(evidence.Question)
	if question == "" || question != evidence.Question || !utf8.ValidString(question) ||
		strings.ContainsRune(question, '\x00') ||
		len(question) > MaxSourceBodyCorrectionQuestionBytes {
		return fmt.Errorf("source-body correction evidence question is invalid")
	}
	wanted := evidence.Question + "\n\n" +
		evidence.BaseCandidate[evidence.StartByte:evidence.EndByte]
	if modelInput != wanted {
		return fmt.Errorf("source-body correction evidence differs from exact model input")
	}
	return nil
}

func (correction SourceBodyCorrection) Validate() error {
	if err := validateSourceBodySpan(
		correction.base, correction.startByte, correction.endByte,
	); err != nil {
		return err
	}
	question := strings.TrimSpace(correction.question)
	if question == "" || question != correction.question || !utf8.ValidString(question) ||
		strings.ContainsRune(question, '\x00') ||
		len(question) > MaxSourceBodyCorrectionQuestionBytes {
		return fmt.Errorf("source-body correction question must be exact bounded UTF-8 text")
	}
	if len(correction.replacements) > 0 {
		if err := validateOpaqueModelChoices(correction.replacements); err != nil {
			return fmt.Errorf("source-body correction replacements: %w", err)
		}
		mutable := correction.Mutable()
		for _, replacement := range correction.replacements {
			if replacement.value == mutable {
				return fmt.Errorf("source-body correction replacement repeats the failed span")
			}
		}
	}
	if correction.identifierReplacement && len(correction.replacements) == 0 {
		return fmt.Errorf(
			"source-body identifier defect has no code-known replacement candidates",
		)
	}
	return nil
}

// ModelInput contains exactly the unresolved question and mutable bytes. It
// cannot render the base source, declaration, path, signature, or persisted
// prompt because none of those values are available through this projection.
func (correction SourceBodyCorrection) ModelInput() (string, error) {
	if err := correction.Validate(); err != nil {
		return "", err
	}
	question, err := correction.modelQuestion()
	if err != nil {
		return "", err
	}
	prompt := question + "\n\n" + correction.Mutable()
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf("source-body correction input exceeds %d bytes", maxPortableResourceBytes)
	}
	return prompt, nil
}

func (correction SourceBodyCorrection) modelQuestion() (string, error) {
	if len(correction.replacements) == 1 {
		return "", fmt.Errorf(
			"source-body correction has one code-owned replacement and forbids a model call",
		)
	}
	if len(correction.replacements) < 2 {
		return correction.question, nil
	}
	rendered, err := RenderOpaqueModelChoiceQuestion(
		correction.question+" The unavailable reference is shown after the choices.",
		nil,
		correction.replacements,
	)
	if err != nil {
		return "", err
	}
	if len(rendered) > MaxSourceBodyCorrectionQuestionBytes {
		return "", fmt.Errorf(
			"source-body correction question exceeds %d bytes",
			MaxSourceBodyCorrectionQuestionBytes,
		)
	}
	return rendered, nil
}

// ApplySoleReplacement performs the zero-inference branch for an identifier
// defect. It returns resolved=false for ordinary source spans or a genuine
// multi-option semantic choice.
func (correction SourceBodyCorrection) ApplySoleReplacement() (
	result string,
	resolved bool,
	err error,
) {
	if err := correction.Validate(); err != nil {
		return "", false, err
	}
	if len(correction.replacements) != 1 {
		return "", false, nil
	}
	result, err = correction.applyReplacement(correction.replacements[0].value)
	return result, err == nil, err
}

func (correction SourceBodyCorrection) OpaqueResponseMaximumBytes() (
	maximum int,
	opaque bool,
	err error,
) {
	if err := correction.Validate(); err != nil {
		return 0, false, err
	}
	if len(correction.replacements) < 2 {
		return 0, false, nil
	}
	maximum, err = opaqueModelChoiceResponseMaximum(correction.replacements)
	return maximum, err == nil, err
}

func (correction SourceBodyCorrection) Mutable() string {
	if correction.startByte < 0 || correction.endByte > len(correction.base) ||
		correction.startByte >= correction.endByte {
		return ""
	}
	return correction.base[correction.startByte:correction.endByte]
}

func (correction SourceBodyCorrection) MutableRange() (int, int, error) {
	if err := correction.Validate(); err != nil {
		return 0, 0, err
	}
	return correction.startByte, correction.endByte, nil
}

func (correction SourceBodyCorrection) BaseCandidate() string {
	return correction.base
}

func (correction SourceBodyCorrection) Evidence() (SourceBodyCorrectionEvidence, error) {
	if err := correction.Validate(); err != nil {
		return SourceBodyCorrectionEvidence{}, err
	}
	question, err := correction.modelQuestion()
	if err != nil {
		return SourceBodyCorrectionEvidence{}, err
	}
	return SourceBodyCorrectionEvidence{
		BaseCandidate:  correction.base,
		BaseSHA256:     sourceBodySHA256(correction.base),
		StartByte:      correction.startByte,
		EndByte:        correction.endByte,
		Question:       question,
		QuestionSHA256: sourceBodySHA256(question),
	}, nil
}

func (correction SourceBodyCorrection) Apply(rawReplacement string) (string, error) {
	result, _, _, err := correction.ApplyWithReplacementRange(rawReplacement)
	return result, err
}

// ApplyWithReplacementRange returns the exact range occupied by the decoded
// replacement in the new body. Callers use it only to prevent a failed model
// replacement from authorizing another correction over its own prose.
func (correction SourceBodyCorrection) ApplyWithReplacementRange(
	rawReplacement string,
) (result string, replacementStart int, replacementEnd int, err error) {
	if err := correction.Validate(); err != nil {
		return "", 0, 0, err
	}
	if len(correction.replacements) == 1 {
		return "", 0, 0, fmt.Errorf(
			"source-body correction has one code-owned replacement and forbids a model response",
		)
	}
	replacement := rawReplacement
	if len(correction.replacements) > 1 {
		replacement, err = DecodeOpaqueModelChoice(rawReplacement, correction.replacements)
	} else {
		candidate, extractErr := sourcebodyresponse.ExtractCandidate(
			rawReplacement, MaxPortableRawCandidateBytes,
		)
		if extractErr != nil {
			err = fmt.Errorf("source-span replacement extraction: %w", extractErr)
		} else {
			replacement, err = NormalizeSourceBodyResponse(candidate.Source)
		}
		if err != nil {
			err = fmt.Errorf("source-span replacement: %w", err)
		}
	}
	if err != nil {
		return "", 0, 0, err
	}
	result, err = correction.applyReplacement(replacement)
	if err != nil {
		return "", 0, 0, err
	}
	return result, correction.startByte, correction.startByte + len(replacement), nil
}

func (correction SourceBodyCorrection) applyReplacement(replacement string) (string, error) {
	if replacement == "" || !utf8.ValidString(replacement) ||
		strings.ContainsRune(replacement, '\x00') {
		return "", fmt.Errorf("source-span replacement is invalid")
	}
	result := correction.base[:correction.startByte] + replacement +
		correction.base[correction.endByte:]
	if len(result) > MaxSourceBodyResponseBytes {
		return "", fmt.Errorf("corrected source body exceeds %d bytes", MaxSourceBodyResponseBytes)
	}
	return result, nil
}

func validateSourceBodySpan(source string, startByte, endByte int) error {
	if source == "" || len(source) > MaxSourceBodyResponseBytes || !utf8.ValidString(source) ||
		strings.ContainsRune(source, '\x00') {
		return fmt.Errorf("source-body span requires bounded code-owned UTF-8 source")
	}
	if startByte < 0 || endByte <= startByte || endByte > len(source) ||
		!utf8.ValidString(source[:startByte]) || !utf8.ValidString(source[:endByte]) {
		return fmt.Errorf("source-body span must be one exact non-empty UTF-8 byte range")
	}
	if startByte == 0 && endByte == len(source) {
		return fmt.Errorf(
			"source-body correction cannot reopen the complete previously returned body",
		)
	}
	return nil
}

func sourceBodySHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
