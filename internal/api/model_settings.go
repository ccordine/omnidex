package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type modelSettingField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	EnvKeys     []string `json:"env_keys"`
	Value       string   `json:"value"`
}

var modelSettingDefinitions = []struct {
	Key         string
	Label       string
	Description string
	EnvKeys     []string
}{
	{Key: "default_model", Label: "Default model", Description: "Primary conversation/responder model", EnvKeys: []string{"OMNI_MODEL", "OMNI_CONVERSATION_MODEL", "OLLAMA_MODEL_RESPONDER", "OLLAMA_MODEL"}},
	{Key: "planner_model", Label: "Planner model", Description: "Structured command planner", EnvKeys: []string{"OMNI_PLANNER_MODEL", "OMNI_STRUCTURED_PLANNER_MODEL", "OLLAMA_MODEL_PLANNER"}},
	{Key: "thinking_model", Label: "Thinking model", Description: "Internal thinking pilot channel", EnvKeys: []string{"OMNI_THINKING_MODEL", "OLLAMA_MODEL_THINKING", "OLLAMA_MODEL_REASONING"}},
	{Key: "evaluator_model", Label: "Evaluator model", Description: "Structured response evaluator", EnvKeys: []string{"OMNI_EVALUATOR_MODEL", "OLLAMA_MODEL_EVALUATOR"}},
	{Key: "shell_specialist_model", Label: "Shell specialist", Description: "Shell execution specialist", EnvKeys: []string{"OMNI_SHELL_SPECIALIST_MODEL", "OLLAMA_MODEL_SPECIALIST_SHELL_EXECUTION", "OLLAMA_MODEL_SHELL"}},
	{Key: "ollama_endpoint", Label: "Ollama endpoint", Description: "Ollama HTTP base URL", EnvKeys: []string{"OLLAMA_BASE_URL", "OMNI_OLLAMA_ENDPOINT"}},
}

func resolveEnvFilePath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("OMNI_ENV_FILE")); explicit != "" {
		return filepath.Abs(explicit)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(cwd, ".env")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	home, err := os.UserHomeDir()
	if err == nil {
		fallback := filepath.Join(home, ".omni", ".env")
		if _, err := os.Stat(fallback); err == nil {
			return fallback, nil
		}
	}
	return candidate, nil
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	return values, scanner.Err()
}

func writeEnvFile(path string, updates map[string]string) error {
	existing, err := readEnvFile(path)
	if err != nil {
		return err
	}
	for key, value := range updates {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		existing[key] = strings.TrimSpace(value)
	}
	keys := make([]string, 0, len(existing))
	for key := range existing {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := []string{"# Updated by Omni GUI model settings"}
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", key, existing[key]))
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func lookupEnvValue(values map[string]string, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func buildModelSettingsResponse() (map[string]any, error) {
	path, err := resolveEnvFilePath()
	if err != nil {
		return nil, err
	}
	values, err := readEnvFile(path)
	if err != nil {
		return nil, err
	}
	fields := make([]modelSettingField, 0, len(modelSettingDefinitions))
	for _, def := range modelSettingDefinitions {
		fields = append(fields, modelSettingField{
			Key:         def.Key,
			Label:       def.Label,
			Description: def.Description,
			EnvKeys:     def.EnvKeys,
			Value:       lookupEnvValue(values, def.EnvKeys),
		})
	}
	return map[string]any{
		"env_file": path,
		"fields":   fields,
	}, nil
}

func (s *Server) handleModelSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		payload, err := buildModelSettingsResponse()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPut:
		var req struct {
			Values map[string]string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		path, err := resolveEnvFilePath()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		updates := map[string]string{}
		for _, def := range modelSettingDefinitions {
			if value, ok := req.Values[def.Key]; ok && len(def.EnvKeys) > 0 {
				updates[def.EnvKeys[0]] = strings.TrimSpace(value)
			}
		}
		if err := writeEnvFile(path, updates); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		payload, err := buildModelSettingsResponse()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
