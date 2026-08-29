package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

const semanticUncertaintyContractIDPrefix = "omnidex.semantic-uncertainty."

// SemanticUncertaintyContract is code-owned justification for one bounded
// semantic call. It is operational evidence only and is never model context.
// Registry lookups return values, so callers cannot mutate the registry.
type SemanticUncertaintyContract struct {
	ID                      string   `json:"id"`
	WorkKind                WorkKind `json:"work_kind"`
	ExactQuestion           string   `json:"exact_question"`
	DeterministicLimitation string   `json:"deterministic_limitation"`
	RequiredInformation     string   `json:"required_information"`
	SingleResult            string   `json:"single_result"`
	DeterministicConsumer   string   `json:"deterministic_consumer"`
}

// SemanticUncertaintyContractForWorkKind resolves the one registered
// uncertainty contract for kind. There is deliberately no generic contract.
func SemanticUncertaintyContractForWorkKind(
	kind WorkKind,
) (SemanticUncertaintyContract, error) {
	contract, ok := registeredSemanticUncertaintyContract(kind)
	if !ok {
		return SemanticUncertaintyContract{}, fmt.Errorf(
			"semantic uncertainty contract for work kind %q is not registered", kind,
		)
	}
	if err := contract.Validate(); err != nil {
		return SemanticUncertaintyContract{}, fmt.Errorf(
			"semantic uncertainty contract for %q: %w", kind, err,
		)
	}
	return contract, nil
}

// SemanticUncertaintyContractForPortableRenderer resolves the exact immutable
// uncertainty contract admitted by the sole current renderer.
func SemanticUncertaintyContractForPortableRenderer(
	renderer string,
	kind WorkKind,
) (SemanticUncertaintyContract, error) {
	if renderer != PortableRendererV1 {
		return SemanticUncertaintyContract{}, fmt.Errorf(
			"portable renderer %q has no registered semantic uncertainty contracts",
			renderer,
		)
	}
	return SemanticUncertaintyContractForWorkKind(kind)
}

func registeredSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	resolvers := [...]func(WorkKind) (SemanticUncertaintyContract, bool){
		applicationSemanticUncertaintyContract,
		repositorySemanticUncertaintyContract,
		databaseSemanticUncertaintyContract,
		webSemanticUncertaintyContract,
		codingSemanticUncertaintyContract,
	}
	for _, resolve := range resolvers {
		if contract, ok := resolve(kind); ok {
			return contract, true
		}
	}
	return SemanticUncertaintyContract{}, false
}

func semanticUncertaintyContract(
	kind WorkKind,
	question string,
	limitation string,
	requiredInformation string,
	singleResult string,
	consumer string,
) SemanticUncertaintyContract {
	return semanticUncertaintyContractVersion(
		kind, "v1", question, limitation, requiredInformation, singleResult, consumer,
	)
}

func semanticUncertaintyContractV2(
	kind WorkKind,
	question string,
	limitation string,
	requiredInformation string,
	singleResult string,
	consumer string,
) SemanticUncertaintyContract {
	return semanticUncertaintyContractVersion(
		kind, "v2", question, limitation, requiredInformation, singleResult, consumer,
	)
}

func semanticUncertaintyContractV3(
	kind WorkKind,
	question string,
	limitation string,
	requiredInformation string,
	singleResult string,
	consumer string,
) SemanticUncertaintyContract {
	return semanticUncertaintyContractVersion(
		kind, "v3", question, limitation, requiredInformation, singleResult, consumer,
	)
}

func semanticUncertaintyContractVersion(
	kind WorkKind,
	version string,
	question string,
	limitation string,
	requiredInformation string,
	singleResult string,
	consumer string,
) SemanticUncertaintyContract {
	return SemanticUncertaintyContract{
		ID:       semanticUncertaintyContractIDPrefix + string(kind) + "." + version,
		WorkKind: kind, ExactQuestion: question,
		DeterministicLimitation: limitation,
		RequiredInformation:     requiredInformation,
		SingleResult:            singleResult,
		DeterministicConsumer:   consumer,
	}
}

func (contract SemanticUncertaintyContract) Validate() error {
	if !validWorkKind(contract.WorkKind) {
		return fmt.Errorf("work kind %q is unsupported", contract.WorkKind)
	}
	fields := []struct {
		name  string
		value string
	}{
		{"exact question", contract.ExactQuestion},
		{"deterministic limitation", contract.DeterministicLimitation},
		{"required information", contract.RequiredInformation},
		{"single result", contract.SingleResult},
		{"deterministic consumer", contract.DeterministicConsumer},
	}
	for _, field := range fields {
		if err := validateSemanticUncertaintyContractField(field.name, field.value); err != nil {
			return err
		}
	}
	if strings.Count(contract.ExactQuestion, "?") != 1 ||
		!strings.HasSuffix(contract.ExactQuestion, "?") {
		return fmt.Errorf("exact question must contain one terminal question mark")
	}
	if !strings.HasPrefix(contract.SingleResult, "One ") {
		return fmt.Errorf("single result must begin with %q", "One ")
	}
	for _, value := range []string{
		contract.ExactQuestion,
		contract.DeterministicLimitation,
		contract.RequiredInformation,
		contract.SingleResult,
		contract.DeterministicConsumer,
	} {
		if term := forbiddenSemanticUncertaintyLanguage(value); term != "" {
			return fmt.Errorf("contract contains forbidden general authority language %q", term)
		}
	}
	registered, ok := registeredSemanticUncertaintyContractByID(
		contract.WorkKind, contract.ID,
	)
	if !ok {
		return fmt.Errorf(
			"work kind %q has no registered contract ID %q",
			contract.WorkKind, contract.ID,
		)
	}
	if contract != registered {
		return fmt.Errorf("contract differs from the exact code-owned registry value")
	}
	return nil
}

func registeredSemanticUncertaintyContractByID(
	kind WorkKind,
	id string,
) (SemanticUncertaintyContract, bool) {
	if current, ok := registeredSemanticUncertaintyContract(kind); ok && current.ID == id {
		return current, true
	}
	return SemanticUncertaintyContract{}, false
}

func validateSemanticUncertaintyContractField(name, value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must be non-empty and exactly trimmed", name)
	}
	if len(value) > 512 || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s must be one bounded UTF-8 line", name)
	}
	return nil
}

func forbiddenSemanticUncertaintyLanguage(value string) string {
	framed := " " + strings.ToLower(value) + " "
	for _, term := range []string{
		" agent ", " agents ", " worker ", " workers ", " orchestrator ",
		" tool call ", " tool calls ", " tool choice ", " tool selection ",
		" control plane ", " workflow decision ", " permission to continue ",
		" approval to continue ", " completion status ", " retry decision ",
	} {
		if strings.Contains(framed, term) {
			return strings.TrimSpace(term)
		}
	}
	return ""
}

// Digest binds the registered identity and all five answers into stable
// evidence bytes. Invalid or locally modified copies cannot be digested.
func (contract SemanticUncertaintyContract) Digest() (string, error) {
	if err := contract.Validate(); err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, value := range []string{
		contract.ID,
		string(contract.WorkKind),
		contract.ExactQuestion,
		contract.DeterministicLimitation,
		contract.RequiredInformation,
		contract.SingleResult,
		contract.DeterministicConsumer,
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
