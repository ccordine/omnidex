package worker

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (s *directCodingSession) validateProgramSource(path, content string) error {
	if s.program == nil || s.specification == nil {
		return fmt.Errorf("coding program validation requires accepted typed semantics and a compiled adapter")
	}
	return validateDirectCodingTypeScriptProgramSource(path, content, *s.program)
}

func validateDirectCodingTypeScriptProgramSource(path, content string, program directCodingProgram) error {
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return err
	}
	expected := ""
	for _, file := range assembly.Files {
		if file.Path == path {
			expected = file.Content
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("adapter %s emitted undeclared source path %s", program.Adapter, path)
	}
	if content != expected {
		return fmt.Errorf("adapter %s source %s differs from its parser-validated in-memory authority", program.Adapter, path)
	}
	extension := strings.ToLower(filepath.Ext(path))
	if path == ".gitignore" {
		return nil
	}
	switch extension {
	case ".ts", ".tsx":
		if err := assemblyline.ValidateTypeScriptSource(content, extension == ".tsx"); err != nil {
			return fmt.Errorf("parse adapter TypeScript source %s: %w", path, err)
		}
	case ".json":
		if !json.Valid([]byte(content)) {
			return fmt.Errorf("adapter %s emitted invalid JSON in %s", program.Adapter, path)
		}
	case ".html":
		if !strings.Contains(content, `id="root"`) || !strings.Contains(content, `/src/main.tsx`) {
			return fmt.Errorf("adapter %s HTML entrypoint %s lacks its required root or module", program.Adapter, path)
		}
	case ".css":
		if strings.TrimSpace(content) == "" {
			return fmt.Errorf("adapter %s emitted empty stylesheet %s", program.Adapter, path)
		}
	default:
		return fmt.Errorf("adapter %s emitted unsupported source extension %q", program.Adapter, extension)
	}
	return nil
}
