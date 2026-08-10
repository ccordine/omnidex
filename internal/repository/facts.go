package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const AnalysisSchemaV1 = "omnidex.repository-analysis.v1"

type FactOrigin string

const (
	OriginGoAST      FactOrigin = "go_ast"
	OriginGoTypes    FactOrigin = "go_types"
	OriginGoPackages FactOrigin = "go_packages"
)

type AdapterIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Symbol struct {
	ID            string          `json:"id"`
	FileID        string          `json:"file_id"`
	Kind          string          `json:"kind"`
	Name          string          `json:"name"`
	QualifiedName string          `json:"qualified_name"`
	Signature     string          `json:"signature"`
	StartByte     int64           `json:"start_byte"`
	EndByte       int64           `json:"end_byte"`
	SourceSHA256  string          `json:"source_sha256"`
	Origin        FactOrigin      `json:"origin"`
	Adapter       AdapterIdentity `json:"adapter"`
	Confidence    float64         `json:"confidence"`
}

type Artifact struct {
	ID           string            `json:"id"`
	FileID       string            `json:"file_id,omitempty"`
	Kind         string            `json:"kind"`
	Name         string            `json:"name"`
	Detail       map[string]string `json:"detail"`
	SourceSHA256 string            `json:"source_sha256"`
	Origin       FactOrigin        `json:"origin"`
	Adapter      AdapterIdentity   `json:"adapter"`
}

type Edge struct {
	ID                string          `json:"id"`
	FromID            string          `json:"from_id"`
	ToID              string          `json:"to_id"`
	Kind              string          `json:"kind"`
	EvidenceFileID    string          `json:"evidence_file_id,omitempty"`
	EvidenceStartByte int64           `json:"evidence_start_byte,omitempty"`
	EvidenceEndByte   int64           `json:"evidence_end_byte,omitempty"`
	Origin            FactOrigin      `json:"origin"`
	Adapter           AdapterIdentity `json:"adapter"`
	Confidence        float64         `json:"confidence"`
}

type AnalysisDiagnostic struct {
	Severity string `json:"severity"`
	Subject  string `json:"subject"`
	Detail   string `json:"detail"`
}

type Analysis struct {
	Schema      string               `json:"schema"`
	ID          string               `json:"id"`
	SnapshotID  string               `json:"snapshot_id"`
	Adapter     AdapterIdentity      `json:"adapter"`
	Complete    bool                 `json:"complete"`
	GeneratedAt time.Time            `json:"generated_at"`
	Symbols     []Symbol             `json:"symbols"`
	Artifacts   []Artifact           `json:"artifacts"`
	Edges       []Edge               `json:"edges"`
	Diagnostics []AnalysisDiagnostic `json:"diagnostics"`
}

type analysisIdentity struct {
	Schema      string               `json:"schema"`
	SnapshotID  string               `json:"snapshot_id"`
	Adapter     AdapterIdentity      `json:"adapter"`
	Complete    bool                 `json:"complete"`
	Symbols     []Symbol             `json:"symbols"`
	Artifacts   []Artifact           `json:"artifacts"`
	Edges       []Edge               `json:"edges"`
	Diagnostics []AnalysisDiagnostic `json:"diagnostics"`
}

func NewSymbol(
	snapshot Snapshot,
	file File,
	adapter AdapterIdentity,
	kind, name, qualifiedName, signature string,
	startByte, endByte int64,
	origin FactOrigin,
	confidence float64,
) Symbol {
	return Symbol{
		ID: opaqueID("symbol_", snapshot.ID, file.ID, kind, qualifiedName,
			fmt.Sprintf("%d:%d", startByte, endByte)),
		FileID: file.ID, Kind: kind, Name: name, QualifiedName: qualifiedName,
		Signature: signature, StartByte: startByte, EndByte: endByte,
		SourceSHA256: file.SHA256, Origin: origin, Adapter: adapter, Confidence: confidence,
	}
}

func NewSnapshotArtifact(
	snapshot Snapshot,
	kind, name string,
	detail map[string]string,
	origin FactOrigin,
	adapter AdapterIdentity,
) (Artifact, error) {
	if err := snapshot.Validate(); err != nil {
		return Artifact{}, fmt.Errorf("snapshot-bound repository artifact: %w", err)
	}
	return newArtifact(snapshot, "", snapshotContentSHA256(snapshot), kind, name, detail, origin, adapter), nil
}

func NewFileArtifact(
	snapshot Snapshot,
	file File,
	kind, name string,
	detail map[string]string,
	origin FactOrigin,
	adapter AdapterIdentity,
) (Artifact, error) {
	if err := snapshot.Validate(); err != nil {
		return Artifact{}, fmt.Errorf("file-bound repository artifact: %w", err)
	}
	var exact File
	for _, candidate := range snapshot.Files {
		if candidate.ID == file.ID {
			exact = candidate
			break
		}
	}
	if exact.ID == "" || exact != file || file.Kind != EntryRegular {
		return Artifact{}, fmt.Errorf("file-bound repository artifact requires one exact regular snapshot file")
	}
	return newArtifact(snapshot, file.ID, file.SHA256, kind, name, detail, origin, adapter), nil
}

func newArtifact(
	snapshot Snapshot,
	fileID, sourceSHA256, kind, name string,
	detail map[string]string,
	origin FactOrigin,
	adapter AdapterIdentity,
) Artifact {
	cleanDetail := cloneStringMap(detail)
	raw, _ := json.Marshal(cleanDetail)
	return Artifact{
		ID:     opaqueID("artifact_", snapshot.ID, fileID, kind, name, string(raw)),
		FileID: fileID, Kind: kind, Name: name, Detail: cleanDetail,
		SourceSHA256: sourceSHA256, Origin: origin, Adapter: adapter,
	}
}

func snapshotContentSHA256(snapshot Snapshot) string {
	return strings.TrimPrefix(snapshot.ID, "snapshot_")
}

func NewEdge(
	snapshot Snapshot,
	adapter AdapterIdentity,
	fromID, toID, kind, evidenceFileID string,
	startByte, endByte int64,
	origin FactOrigin,
	confidence float64,
) Edge {
	return Edge{
		ID: opaqueID("edge_", snapshot.ID, fromID, toID, kind, evidenceFileID,
			fmt.Sprintf("%d:%d", startByte, endByte), string(origin)),
		FromID: fromID, ToID: toID, Kind: kind, EvidenceFileID: evidenceFileID,
		EvidenceStartByte: startByte, EvidenceEndByte: endByte,
		Origin: origin, Adapter: adapter, Confidence: confidence,
	}
}

func FinalizeAnalysis(analysis *Analysis) error {
	if analysis == nil {
		return fmt.Errorf("repository analysis is required")
	}
	analysis.GeneratedAt = canonicalRepositoryTimestamp(analysis.GeneratedAt)
	sort.Slice(analysis.Symbols, func(left, right int) bool { return analysis.Symbols[left].ID < analysis.Symbols[right].ID })
	sort.Slice(analysis.Artifacts, func(left, right int) bool { return analysis.Artifacts[left].ID < analysis.Artifacts[right].ID })
	sort.Slice(analysis.Edges, func(left, right int) bool { return analysis.Edges[left].ID < analysis.Edges[right].ID })
	sort.Slice(analysis.Diagnostics, func(left, right int) bool {
		if analysis.Diagnostics[left].Subject == analysis.Diagnostics[right].Subject {
			return analysis.Diagnostics[left].Detail < analysis.Diagnostics[right].Detail
		}
		return analysis.Diagnostics[left].Subject < analysis.Diagnostics[right].Subject
	})
	id, err := analysisID(*analysis)
	if err != nil {
		return err
	}
	analysis.ID = id
	return nil
}

func (analysis Analysis) Validate(snapshot Snapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("repository analysis snapshot: %w", err)
	}
	if analysis.Schema != AnalysisSchemaV1 || analysis.SnapshotID != snapshot.ID {
		return fmt.Errorf("repository analysis is not bound to the exact snapshot")
	}
	if err := analysis.Adapter.validate(); err != nil {
		return err
	}
	if err := validateCanonicalGeneratedAt("repository analysis", analysis.GeneratedAt); err != nil {
		return err
	}
	if analysis.Symbols == nil || analysis.Artifacts == nil || analysis.Edges == nil || analysis.Diagnostics == nil {
		return fmt.Errorf("repository analysis facts require canonical non-nil collections")
	}
	files := make(map[string]File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	if err := validateSymbols(analysis.Symbols, files, analysis.Adapter); err != nil {
		return err
	}
	if err := validateArtifacts(analysis.Artifacts, files, snapshot, analysis.Adapter); err != nil {
		return err
	}
	endpoints := make(map[string]struct{}, len(analysis.Symbols)+len(analysis.Artifacts))
	for _, symbol := range analysis.Symbols {
		endpoints[symbol.ID] = struct{}{}
	}
	for _, artifact := range analysis.Artifacts {
		endpoints[artifact.ID] = struct{}{}
	}
	if err := validateEdges(analysis.Edges, files, endpoints, analysis.Adapter); err != nil {
		return err
	}
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Severity != "warning" && diagnostic.Severity != "error" {
			return fmt.Errorf("repository analysis diagnostic has invalid severity %q", diagnostic.Severity)
		}
		if strings.TrimSpace(diagnostic.Subject) == "" || strings.TrimSpace(diagnostic.Detail) == "" {
			return fmt.Errorf("repository analysis diagnostic requires subject and detail")
		}
	}
	if analysis.Complete {
		for _, diagnostic := range analysis.Diagnostics {
			if diagnostic.Severity == "error" {
				return fmt.Errorf("complete repository analysis cannot contain error diagnostics")
			}
		}
	}
	expected, err := analysisID(analysis)
	if err != nil {
		return err
	}
	if analysis.ID != expected {
		return fmt.Errorf("repository analysis ID does not match its exact facts")
	}
	return nil
}

func (identity AdapterIdentity) validate() error {
	if strings.TrimSpace(identity.Name) == "" || strings.TrimSpace(identity.Version) == "" {
		return fmt.Errorf("repository analysis requires an adapter name and version")
	}
	return nil
}

func analysisID(analysis Analysis) (string, error) {
	identity := analysisIdentity{
		Schema: analysis.Schema, SnapshotID: analysis.SnapshotID, Adapter: analysis.Adapter,
		Complete: analysis.Complete, Symbols: analysis.Symbols, Artifacts: analysis.Artifacts,
		Edges: analysis.Edges, Diagnostics: analysis.Diagnostics,
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode repository analysis identity: %w", err)
	}
	hash := sha256.Sum256(raw)
	return "analysis_" + hex.EncodeToString(hash[:]), nil
}

func cloneStringMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
