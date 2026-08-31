package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/version"
)

func writeVersionJSON(output io.Writer) error {
	payload, err := json.MarshalIndent(version.JSON(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode Omnidex version: %w", err)
	}
	payload = append(payload, '\n')
	if _, err := output.Write(payload); err != nil {
		return fmt.Errorf("write Omnidex version: %w", err)
	}
	return nil
}
