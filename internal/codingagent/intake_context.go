package codingagent

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func (context IntakeContext) Validate() error {
	if strings.TrimSpace(context.RedactedRequest) == "" || !utf8.ValidString(context.RedactedRequest) || strings.ContainsRune(context.RedactedRequest, 0) {
		return fmt.Errorf("intake context requires its exact nonempty redacted request")
	}
	tokens := map[string]bool{}
	for _, binding := range context.ArtifactBindings {
		if binding.Token == "" || strings.TrimSpace(binding.Token) != binding.Token || tokens[binding.Token] || !utf8.ValidString(binding.Token) || !utf8.ValidString(binding.Value) || strings.ContainsAny(binding.Token, "\x00\r\n") || strings.ContainsRune(binding.Value, 0) {
			return fmt.Errorf("intake artifact binding lacks its exact unique token and valid value")
		}
		tokens[binding.Token] = true
	}
	return nil
}
