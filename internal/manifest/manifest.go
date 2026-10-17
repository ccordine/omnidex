// Package manifest loads installation defaults shared by the CLI and server.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/modelconfig"
)

const DefaultCoreURL = "https://core.omni.worknet"

type Models struct {
	Default  string            `json:"default"`
	Planner  codingagent.Route `json:"planner"`
	Executor codingagent.Route `json:"executor"`
	Reviewer codingagent.Route `json:"reviewer"`
	Parsers  codingagent.Route `json:"parsers"`
}

type Manifest struct {
	CoreURL string `json:"core_url"`
	Models  Models `json:"models"`
}

func Defaults() Manifest {
	low := codingagent.Route{Provider: "codex", Model: "gpt-6.1-sol", Effort: "low", ServiceTier: "default"}
	xhigh := low
	xhigh.Effort = "xhigh"
	return Manifest{CoreURL: DefaultCoreURL, Models: Models{
		Default: low.Model, Planner: xhigh, Executor: low, Reviewer: xhigh, Parsers: low,
	}}
}

func HomePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".omni", "manifest.json"), nil
}

// LoadHome leaves installations without a manifest on their existing explicit
// configuration. A present invalid manifest is an error, never ignored.
func LoadHome() (Manifest, bool, error) {
	path, err := HomePath()
	if err != nil {
		return Manifest{}, false, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	value, err := Decode(raw)
	if err != nil {
		return Manifest{}, false, fmt.Errorf("manifest %s: %w", path, err)
	}
	return value, true, nil
}

func Decode(raw []byte) (Manifest, error) {
	var value Manifest
	if err := exactjson.ValidateObject(raw, &value, "Omnidex manifest"); err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	parsed, err := url.Parse(value.CoreURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return value, fmt.Errorf("core_url requires an absolute HTTP or HTTPS URL")
	}
	if strings.TrimSpace(value.Models.Default) == "" {
		return value, fmt.Errorf("models.default is required")
	}
	if err := value.Coding().Validate(); err != nil {
		return value, err
	}
	for _, route := range []codingagent.Route{value.Models.Planner, value.Models.Executor, value.Models.Reviewer, value.Models.Parsers} {
		if route.Effort == "" || route.ServiceTier == "" {
			return value, fmt.Errorf("every model route requires explicit effort and service_tier")
		}
	}
	return value, nil
}

func (value Manifest) Coding() codingagent.Configuration {
	return codingagent.Configuration{Mode: codingagent.Agents, Planner: value.Models.Planner,
		Executor: value.Models.Executor, Reviewer: value.Models.Reviewer, LedgerParser: value.Models.Parsers}
}

func (value Manifest) ModelAuthority() (modelconfig.Authority, error) {
	models := modelconfig.Config{}
	for _, field := range modelconfig.RegisteredFields() {
		models[field.Key] = value.Models.Default
	}
	return modelconfig.Freeze(models)
}
