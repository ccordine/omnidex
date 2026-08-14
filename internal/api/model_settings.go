package api

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/modelconfig"
)

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
	if err := modelconfig.ValidateEnvironmentValues(existing); err != nil {
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

func buildModelSettingsResponse() (map[string]any, error) {
	path, err := resolveEnvFilePath()
	if err != nil {
		return nil, err
	}
	values, err := readEnvFile(path)
	if err != nil {
		return nil, err
	}
	if err := modelconfig.ValidateEnvironmentValues(values); err != nil {
		return nil, err
	}
	fields := (modelconfig.Config{}).FieldList(values)
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
		req, err := decodeModelSettingsRequest(w, r)
		if err != nil {
			writeError(w, exactSettingsErrorStatus(err), err.Error())
			return
		}
		if err := validateModelSettingKeys(req.Values); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		path, err := resolveEnvFilePath()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		updates := map[string]string{}
		for _, def := range modelconfig.Fields {
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

func validateModelSettingKeys(values map[string]string) error {
	allowed := make(map[string]struct{}, len(modelconfig.Fields))
	for _, field := range modelconfig.Fields {
		allowed[field.Key] = struct{}{}
	}
	unknown := make([]string, 0)
	for key := range values {
		if _, ok := allowed[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("model settings contain unsupported field %q", unknown[0])
}
