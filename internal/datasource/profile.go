package datasource

import (
	"strings"
	"time"
)

const (
	DriverPostgres = "postgres"
	DomainGeneric  = "generic"
	DomainHealthcare = "healthcare"
	DomainGaming   = "gaming"
	DomainAnalytics = "analytics"

	PrivacyStrict = "strict"
)

// Connection profile fields stored on each data source record.
type Profile struct {
	Driver        string `json:"driver"`
	Domain        string `json:"domain"`
	ContextPrompt string `json:"context_prompt"`
	PrivacyMode   string `json:"privacy_mode"`
}

func NormalizeProfile(p Profile) Profile {
	p.Driver = strings.ToLower(strings.TrimSpace(p.Driver))
	if p.Driver == "" {
		p.Driver = DriverPostgres
	}
	p.Domain = strings.ToLower(strings.TrimSpace(p.Domain))
	if p.Domain == "" {
		p.Domain = DomainGeneric
	}
	p.ContextPrompt = strings.TrimSpace(p.ContextPrompt)
	p.PrivacyMode = strings.ToLower(strings.TrimSpace(p.PrivacyMode))
	if p.PrivacyMode == "" {
		p.PrivacyMode = PrivacyStrict
	}
	return p
}

func DomainGuidance(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case DomainHealthcare:
		return strings.Join([]string{
			"Healthcare/clinical database. Prefer aggregate counts and risk cohorts.",
			"Never return patient names, MRNs, SSNs, addresses, phone numbers, or clinical progress notes in query results.",
			"Patient-submitted comments, portal feedback, and survey responses may be analyzed for sentiment/themes using bounded samples (LIMIT <= 24); staff see aggregate themes and counts, not raw comment text.",
			"Use de-identified keys only when necessary.",
		}, " ")
	case DomainGaming:
		return "Game analytics database. Player stats, sessions, inventories, and match outcomes are expected. Prefer player_id over display names when both exist."
	case DomainAnalytics:
		return "General analytics warehouse. Prefer summarized metrics and time-bucketed aggregates."
	default:
		return "General read-only SQL analytics. Prefer aggregates over wide row dumps."
	}
}

type ColumnCatalog struct {
	Name        string `json:"name"`
	DataType    string `json:"data_type"`
	Nullable    bool   `json:"nullable"`
	Sensitive   bool   `json:"sensitive"`
	TextContent bool   `json:"text_content,omitempty"`
	Hint        string `json:"hint,omitempty"`
}

type TableCatalog struct {
	Schema      string          `json:"schema"`
	Name        string          `json:"name"`
	FullName    string          `json:"full_name"`
	Purpose     string          `json:"purpose,omitempty"`
	RowEstimate int64           `json:"row_estimate,omitempty"`
	Columns     []ColumnCatalog `json:"columns"`
}

type SchemaCatalog struct {
	SourceID    string         `json:"source_id"`
	SourceName  string         `json:"source_name"`
	Driver      string         `json:"driver"`
	Domain      string         `json:"domain"`
	Fingerprint string         `json:"fingerprint"`
	Tables      []TableCatalog `json:"tables"`
	Summary     string         `json:"summary,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (c SchemaCatalog) TableByName(fullName string) (TableCatalog, bool) {
	fullName = strings.ToLower(strings.TrimSpace(fullName))
	for _, table := range c.Tables {
		if strings.ToLower(table.FullName) == fullName {
			return table, true
		}
	}
	return TableCatalog{}, false
}
