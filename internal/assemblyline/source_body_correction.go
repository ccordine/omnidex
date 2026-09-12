package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/sourcebodyresponse"
)

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
	BaseCandidate string
	StartByte     int
	EndByte       int
	Question      string
}

func (evidence SourceBodyCorrectionEvidence) Validate(modelInput string) error {
	if err := validateSourceBodySpan(
		evidence.BaseCandidate, evidence.StartByte, evidence.EndByte,
	); err != nil {
		return err
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
		BaseCandidate: correction.base,
		StartByte:     correction.startByte,
		EndByte:       correction.endByte,
		Question:      question,
	}, nil
}

func (correction SourceBodyCorrection) Apply(rawReplacement string) (string, error) {
	if err := correction.Validate(); err != nil {
		return "", err
	}
	if len(correction.replacements) == 1 {
		return "", fmt.Errorf(
			"source-body correction has one code-owned replacement and forbids a model response",
		)
	}
	replacement := rawReplacement
	if len(correction.replacements) > 1 {
		decoded, err := DecodeOpaqueModelChoice(rawReplacement, correction.replacements)
		if err != nil {
			return "", err
		}
		replacement = decoded
	} else {
		candidate, extractErr := sourcebodyresponse.ExtractCandidate(
			rawReplacement, MaxPortableRawCandidateBytes,
		)
		if extractErr != nil {
			return "", fmt.Errorf("source-span replacement extraction: %w", extractErr)
		}
		normalized, err := NormalizeSourceBodyResponse(candidate.Source)
		if err != nil {
			return "", fmt.Errorf("source-span replacement: %w", err)
		}
		replacement = normalized
	}
	return correction.applyReplacement(replacement)
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
