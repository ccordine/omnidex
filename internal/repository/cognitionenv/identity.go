package cognitionenv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func NewInvestigation(
	projectID int64,
	snapshot repository.Snapshot,
	analysis repository.Analysis,
	need NeedAuthority,
	operation repositoryretrieval.Operation,
	query string,
) (Investigation, error) {
	if projectID < 1 {
		return Investigation{}, fmt.Errorf("repository cognition requires positive project authority")
	}
	if err := snapshot.Validate(); err != nil {
		return Investigation{}, fmt.Errorf("repository cognition snapshot: %w", err)
	}
	if err := analysis.Validate(snapshot); err != nil {
		return Investigation{}, fmt.Errorf("repository cognition analysis: %w", err)
	}
	if !analysis.Complete {
		return Investigation{}, fmt.Errorf("repository cognition requires one complete analysis")
	}
	if err := validateNeedAuthority(need, snapshot); err != nil {
		return Investigation{}, err
	}
	binding, err := repositoryretrieval.NewQueryBinding(operation, query)
	if err != nil {
		return Investigation{}, err
	}
	catalog, err := RegisteredCatalog(operation)
	if err != nil {
		return Investigation{}, err
	}
	predicate, err := cognition.NewPredicate(
		PredicateEvidenceAcquired, []string{string(operation), binding},
	)
	if err != nil {
		return Investigation{}, err
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		return Investigation{}, err
	}
	check := completionCheck(catalog, operation, binding)
	completion, err := cognition.NewCompletionAuthority(
		check, []cognition.PredicateName{PredicateEvidenceAcquired},
	)
	if err != nil {
		return Investigation{}, err
	}
	public := struct {
		Schema       string
		SnapshotID   string
		AnalysisID   string
		Operation    repositoryretrieval.Operation
		QueryBinding string
		Need         NeedAuthority
		CatalogSHA   string
		Goal         cognition.GoalExpression
		Completion   cognition.CompletionAuthority
	}{ScenarioSchemaV2, snapshot.ID, analysis.ID, operation, binding, need, catalog.SHA256, goal, completion}
	digest, err := exactDigest(public)
	if err != nil {
		return Investigation{}, err
	}
	ref, err := cognition.NewScenarioRef(
		cognition.ScenarioID("repository-investigation-"+digest), digest,
	)
	if err != nil {
		return Investigation{}, err
	}
	return Investigation{
		projectID: projectID, snapshot: snapshot, analysis: analysis,
		operation: operation, query: query, queryBinding: binding, need: need,
		ref: ref, goal: goal, catalog: catalog, completion: completion,
	}, nil
}

// RegisteredCatalog is the exact bounded read-only action authority for one
// accepted repository retrieval operation.
func RegisteredCatalog(operation repositoryretrieval.Operation) (cognition.ActionCatalog, error) {
	schemas, err := actionSchemasFor(operation)
	if err != nil {
		return cognition.ActionCatalog{}, err
	}
	return cognition.NewActionCatalog(
		cognition.ActionCatalogID("repository.investigation.v2"), ActionVersionV1, schemas,
	)
}

func NewNeedAuthority(content string) (NeedAuthority, error) {
	if content == "" || content != strings.TrimSpace(content) || len(content) > 4*1024 ||
		!utf8.ValidString(content) || strings.ContainsRune(content, '\x00') ||
		modelcontext.ContainsPathIdentity(content) {
		return NeedAuthority{}, fmt.Errorf("repository cognition need requires 1-4096 exact trimmed UTF-8 bytes")
	}
	digest := textDigest(content)
	return NeedAuthority{
		ID: "repository-need-" + digest, Content: content, ContentSHA256: digest,
	}, nil
}

func validateNeedAuthority(need NeedAuthority, snapshot repository.Snapshot) error {
	want, err := NewNeedAuthority(need.Content)
	if err != nil || need != want {
		return fmt.Errorf("repository cognition need has invalid source authority")
	}
	identities := []string{snapshot.Root}
	for _, file := range snapshot.Files {
		identities = append(identities, file.Path, filepath.Base(file.Path), file.ID)
	}
	for _, identity := range identities {
		if identity != "" && strings.Contains(need.Content, identity) {
			return fmt.Errorf("repository cognition need exposes prohibited path or file identity")
		}
	}
	return nil
}

func actionSchemasFor(operation repositoryretrieval.Operation) ([]cognition.ActionSchema, error) {
	if err := operation.Validate(); err != nil {
		return nil, err
	}
	search, err := cognition.NewActionSchema(
		"repository.search.v2", ActionVersionV1, ActionSearch,
		[]cognition.ActionParameterSpec{}, cognition.EvidenceRequired,
	)
	if err != nil {
		return nil, err
	}
	schemas := []cognition.ActionSchema{search}
	if operation == repositoryretrieval.OperationSemanticExcerpts {
		return schemas, nil
	}
	inspect, err := cognition.NewActionSchema(
		"repository.inspect.v2", ActionVersionV1, ActionInspect,
		[]cognition.ActionParameterSpec{{
			Name: ArgumentSymbolRef, Required: true, MaxBytes: 128,
		}}, cognition.EvidenceRequired,
	)
	if err != nil {
		return nil, err
	}
	schemas = append(schemas, inspect)
	if operation == repositoryretrieval.OperationSymbolDeclaration {
		return schemas, nil
	}
	references, err := cognition.NewActionSchema(
		"repository.references.v2", ActionVersionV1, ActionReferences,
		[]cognition.ActionParameterSpec{{
			Name: ArgumentSymbolRef, Required: true, MaxBytes: 128,
		}}, cognition.EvidenceRequired,
	)
	if err != nil {
		return nil, err
	}
	return append(schemas, references), nil
}

func completionCheck(
	catalog cognition.ActionCatalog,
	operation repositoryretrieval.Operation,
	binding string,
) cognition.CompletionCheckRef {
	digest, err := exactDigest(struct {
		Schema       string
		Predicate    cognition.PredicateName
		Operation    repositoryretrieval.Operation
		QueryBinding string
		CatalogSHA   string
	}{ScenarioSchemaV2, PredicateEvidenceAcquired, operation, binding, catalog.SHA256})
	if err != nil {
		panic(err)
	}
	return cognition.CompletionCheckRef{
		ID: "repository.evidence-acquired.v1", Version: ActionVersionV1, SHA256: digest,
	}
}

func exactDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode repository cognition identity: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func textDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
