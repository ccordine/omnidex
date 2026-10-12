package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/gryph/omnidex/internal/modelconfig"
)

// Installation derives routing from the same registry the runtime consumes.
func writeModelEnvironment(output io.Writer, model string) error {
	if model == "" || strings.ContainsAny(model, " \t\r\n\"'\\") {
		return fmt.Errorf("model-env requires one plain model identifier")
	}
	for _, field := range modelconfig.RegisteredFields() {
		for _, key := range field.EnvKeys {
			if _, err := fmt.Fprintf(output, "%s=%s\n", key, model); err != nil {
				return err
			}
		}
	}
	return nil
}
