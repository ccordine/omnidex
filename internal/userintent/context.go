package userintent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
)

// BuildContext accepts only caller-scoped persisted facts and at most two
// exchanges. The shared reducer handles genuinely large relevant input; neither
// the current message nor the server snapshot is summarized here.
func BuildContext(ctx context.Context, c Context, message string, minify Ask) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(c.Recent) > 4 || len(c.Facts) > 32 {
		return "", fmt.Errorf("user context permits two recent exchanges and 32 scoped facts")
	}
	for _, fact := range c.Facts {
		if err := boundedText(fact, 64<<10); err != nil {
			return "", err
		}
	}
	if c.Pending != "" {
		if err := boundedText(c.Pending, 16<<10); err != nil {
			return "", err
		}
	}
	roles := map[string]int{}
	for _, turn := range c.Recent {
		if turn.Role != "user" && turn.Role != "assistant" {
			return "", fmt.Errorf("unregistered conversation role")
		}
		roles[turn.Role]++
		if roles[turn.Role] > 2 {
			return "", fmt.Errorf("context exceeds two recent exchanges")
		}
		if err := boundedText(turn.Text, 64<<10); err != nil {
			return "", err
		}
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	if len(encoded) > 128<<10 {
		return "", fmt.Errorf("relevant context input exceeds 128 KiB")
	}
	if len(encoded) <= assemblyline.ContextMinificationThresholdBytes {
		return string(encoded), nil
	}
	if minify == nil {
		return "", fmt.Errorf("large relevant context requires minification")
	}
	// Session chunks may repeat verbatim; retain their distinct positions rather
	// than treating repeated persisted text as duplicate independent authorities.
	chunks := []assemblyline.ContextCandidateAuthority{}
	remaining := string(encoded)
	for len(remaining) > 0 {
		n := min(len(remaining), assemblyline.MaxContextCandidateContentBytes)
		for n < len(remaining) && !utf8.RuneStart(remaining[n]) {
			n--
		}
		// Whitespace-only partitions carry no reference or instruction. The
		// shared reducer requires nonblank authorities, so omit those partitions.
		content := remaining[:n]
		remaining = remaining[n:]
		if strings.TrimSpace(content) == "" {
			continue
		}
		chunks = append(chunks, assemblyline.ContextCandidateAuthority{Namespace: "session_user_context", CandidateID: fmt.Sprintf("CTX_%d", len(chunks)+1), Content: content})
	}
	text, err := contextcompiler.ReduceRelevant(ctx, message, chunks, contextMinifier{minify})
	if err != nil {
		return "", err
	}
	encoded, err = json.Marshal(struct {
		Capsule string `json:"capsule"`
	}{text})
	if err != nil {
		return "", err
	}
	if len(encoded) > 24<<10 {
		return "", fmt.Errorf("dedicated user context exceeds 24 KiB after minification")
	}
	return string(encoded), nil
}
func boundedText(value string, maximum int) error {
	if strings.TrimSpace(value) == "" || len(value) > maximum || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("semantic text must be nonblank UTF-8 within %d bytes", maximum)
	}
	return nil
}

type contextMinifier struct{ ask Ask }

func (m contextMinifier) Minify(ctx context.Context, input assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error) {
	prompt, err := assemblyline.BuildContextMinificationPrompt(input)
	if err != nil {
		return assemblyline.ContextMinificationDecision{}, 0, err
	}
	raw, err := m.ask(ctx, prompt)
	if err != nil {
		return assemblyline.ContextMinificationDecision{}, 1, err
	}
	decision, err := assemblyline.DecodeContextMinificationDecision(input, raw)
	return decision, 1, err
}
