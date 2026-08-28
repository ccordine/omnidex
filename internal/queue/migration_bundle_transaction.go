package queue

import (
	"fmt"
	"strings"
)

// transactionalBody returns exact migration semantics for execution inside a
// caller-owned transaction. Only one exact outer wrapper may be removed.
func (entry migrationBundleEntry) transactionalBody() ([]byte, error) {
	body := string(entry.body)
	const begin = "BEGIN;\n"
	const commit = "COMMIT;\n"
	if strings.HasPrefix(body, begin) && strings.HasSuffix(body, commit) {
		body = strings.TrimSuffix(strings.TrimPrefix(body, begin), commit)
	}
	if control, err := nestedMigrationTransactionControl(body); err != nil {
		return nil, fmt.Errorf("migration %q SQL lexical boundary: %w", entry.name, err)
	} else if control != "" {
		return nil, fmt.Errorf(
			"migration %q has unsupported nested transaction control %q",
			entry.name, control,
		)
	}
	return []byte(body), nil
}

// nestedMigrationTransactionControl inspects only statement-leading SQL
// keywords. Quoted values, quoted identifiers, comments, and dollar-quoted
// function bodies cannot manufacture or hide a caller-owned transaction
// boundary.
func nestedMigrationTransactionControl(body string) (string, error) {
	for offset := 0; offset < len(body); {
		start, err := skipMigrationSQLTrivia(body, offset)
		if err != nil {
			return "", err
		}
		if start >= len(body) {
			return "", nil
		}
		if body[start] == ';' {
			offset = start + 1
			continue
		}

		first, firstEnd := migrationSQLBareWord(body, start)
		if first == "" {
			return "", fmt.Errorf("SQL statement does not begin with one unquoted command keyword")
		}
		upperFirst := strings.ToUpper(first)
		control, err := forbiddenMigrationTransactionControl(body, firstEnd, upperFirst)
		if err != nil {
			return "", err
		}
		if control != "" {
			return control, nil
		}

		offset, err = migrationSQLStatementEnd(body, start)
		if err != nil {
			return "", err
		}
	}
	return "", nil
}

func forbiddenMigrationTransactionControl(body string, offset int, first string) (string, error) {
	switch first {
	case "BEGIN", "COMMIT", "END", "ROLLBACK", "ABORT", "SAVEPOINT", "RELEASE":
		return first, nil
	case "START", "PREPARE":
		second, _, err := nextMigrationSQLBareWord(body, offset)
		if err != nil || second != "TRANSACTION" {
			return "", err
		}
		return first + " TRANSACTION", nil
	case "SET":
		second, secondEnd, err := nextMigrationSQLBareWord(body, offset)
		if err != nil {
			return "", err
		}
		if second == "TRANSACTION" {
			return "SET TRANSACTION", nil
		}
		if second == "LOCAL" {
			third, _, err := nextMigrationSQLBareWord(body, secondEnd)
			if err != nil || third != "TRANSACTION" {
				return "", err
			}
			return "SET LOCAL TRANSACTION", nil
		}
		if second != "SESSION" {
			return "", nil
		}
		third, thirdEnd, err := nextMigrationSQLBareWord(body, secondEnd)
		if err != nil || third != "CHARACTERISTICS" {
			return "", err
		}
		fourth, fourthEnd, err := nextMigrationSQLBareWord(body, thirdEnd)
		if err != nil || fourth != "AS" {
			return "", err
		}
		fifth, _, err := nextMigrationSQLBareWord(body, fourthEnd)
		if err != nil || fifth != "TRANSACTION" {
			return "", err
		}
		return "SET SESSION CHARACTERISTICS AS TRANSACTION", nil
	default:
		return "", nil
	}
}

func nextMigrationSQLBareWord(body string, offset int) (string, int, error) {
	start, err := skipMigrationSQLTrivia(body, offset)
	if err != nil {
		return "", 0, err
	}
	word, end := migrationSQLBareWord(body, start)
	return strings.ToUpper(word), end, nil
}

func migrationSQLBareWord(body string, offset int) (string, int) {
	if offset >= len(body) || !migrationSQLIdentifierStart(body[offset]) {
		return "", offset
	}
	end := offset + 1
	for end < len(body) && migrationSQLIdentifierPart(body[end]) {
		end++
	}
	return body[offset:end], end
}

func migrationSQLIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func migrationSQLIdentifierPart(value byte) bool {
	// PostgreSQL permits non-ASCII letters in unquoted identifiers. Treat every
	// high-bit byte as an identifier continuation here: this lexer need not
	// decode the identifier, but it must never mistake a dollar-tag-shaped
	// suffix inside one for the start of a dollar-quoted value.
	return migrationSQLIdentifierStart(value) || value >= '0' && value <= '9' ||
		value == '$' || value >= 0x80
}

func skipMigrationSQLTrivia(body string, offset int) (int, error) {
	for offset < len(body) {
		switch {
		case migrationSQLSpace(body[offset]):
			offset++
		case strings.HasPrefix(body[offset:], "--"):
			offset += 2
			for offset < len(body) && body[offset] != '\n' && body[offset] != '\r' {
				offset++
			}
		case strings.HasPrefix(body[offset:], "/*"):
			var err error
			offset, err = skipMigrationSQLBlockComment(body, offset)
			if err != nil {
				return 0, err
			}
		default:
			return offset, nil
		}
	}
	return offset, nil
}

func migrationSQLStatementEnd(body string, offset int) (int, error) {
	for offset < len(body) {
		switch {
		case strings.HasPrefix(body[offset:], "--"):
			offset += 2
			for offset < len(body) && body[offset] != '\n' && body[offset] != '\r' {
				offset++
			}
		case strings.HasPrefix(body[offset:], "/*"):
			var err error
			offset, err = skipMigrationSQLBlockComment(body, offset)
			if err != nil {
				return 0, err
			}
		case body[offset] == '\'':
			var err error
			offset, err = skipMigrationSQLQuoted(
				body, offset, '\'', migrationSQLEscapeStringPrefix(body, offset),
			)
			if err != nil {
				return 0, err
			}
		case body[offset] == '"':
			var err error
			offset, err = skipMigrationSQLQuoted(body, offset, '"', false)
			if err != nil {
				return 0, err
			}
		case body[offset] == '$':
			delimiter, found, err := migrationSQLDollarDelimiter(body, offset)
			if err != nil {
				return 0, err
			}
			if !found {
				offset++
				continue
			}
			closing := strings.Index(body[offset+len(delimiter):], delimiter)
			if closing < 0 {
				return 0, fmt.Errorf("unterminated dollar-quoted SQL value")
			}
			offset += len(delimiter) + closing + len(delimiter)
		case body[offset] == ';':
			return offset + 1, nil
		default:
			offset++
		}
	}
	return offset, nil
}

func skipMigrationSQLBlockComment(body string, offset int) (int, error) {
	depth := 0
	for offset < len(body) {
		switch {
		case strings.HasPrefix(body[offset:], "/*"):
			depth++
			offset += 2
		case strings.HasPrefix(body[offset:], "*/"):
			depth--
			offset += 2
			if depth == 0 {
				return offset, nil
			}
		default:
			offset++
		}
	}
	return 0, fmt.Errorf("unterminated SQL block comment")
}

func skipMigrationSQLQuoted(body string, offset int, quote byte, backslashEscapes bool) (int, error) {
	for offset++; offset < len(body); {
		if backslashEscapes && body[offset] == '\\' {
			offset += 2
			continue
		}
		if body[offset] != quote {
			offset++
			continue
		}
		if offset+1 < len(body) && body[offset+1] == quote {
			offset += 2
			continue
		}
		return offset + 1, nil
	}
	return 0, fmt.Errorf("unterminated quoted SQL value")
}

func migrationSQLEscapeStringPrefix(body string, quote int) bool {
	if quote == 0 || body[quote-1] != 'E' && body[quote-1] != 'e' {
		return false
	}
	return quote == 1 || !migrationSQLIdentifierPart(body[quote-2])
}

func migrationSQLDollarDelimiter(body string, offset int) (string, bool, error) {
	if offset+1 >= len(body) {
		return "", false, nil
	}
	if offset > 0 && migrationSQLIdentifierPart(body[offset-1]) {
		return "", false, nil
	}
	if body[offset+1] == '$' {
		return "$$", true, nil
	}
	if body[offset+1] >= 0x80 {
		return "", false, fmt.Errorf("non-ASCII dollar-quote tag is unsupported")
	}
	if !migrationSQLIdentifierStart(body[offset+1]) {
		return "", false, nil
	}
	end := offset + 2
	for end < len(body) {
		switch {
		case migrationSQLIdentifierStart(body[end]) ||
			body[end] >= '0' && body[end] <= '9':
			end++
		case body[end] >= 0x80:
			return "", false, fmt.Errorf("non-ASCII dollar-quote tag is unsupported")
		default:
			if body[end] != '$' {
				return "", false, nil
			}
			return body[offset : end+1], true, nil
		}
	}
	return "", false, nil
}

func migrationSQLSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' ||
		value == '\f' || value == '\v'
}

func validateMigrationTransactionControl(entries []migrationBundleEntry) error {
	for index, entry := range entries {
		if _, err := entry.transactionalBody(); err != nil {
			return fmt.Errorf("migration bundle entry %d transaction control: %w", index, err)
		}
	}
	return nil
}
