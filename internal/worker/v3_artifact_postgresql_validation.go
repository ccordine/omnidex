package worker

import (
	"fmt"
	"strings"
)

func validatePostgreSQLMigrationArtifactSource(artifactPath string, source []byte) error {
	if _, recognized := postgreSQLMigrationArtifactRecognizer(artifactPath); !recognized {
		return fmt.Errorf("PostgreSQL migration path is outside database/migrations")
	}
	trimmed := strings.TrimSpace(string(source))
	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "BEGIN;") || !strings.HasSuffix(upper, "COMMIT;") {
		return fmt.Errorf("PostgreSQL migration requires one explicit BEGIN/COMMIT boundary")
	}
	if !strings.Contains(upper, "CREATE TABLE IF NOT EXISTS ") {
		return fmt.Errorf("PostgreSQL migration requires idempotent table ownership")
	}
	for _, forbidden := range []string{
		"ALTER SYSTEM", "COPY PROGRAM", "CREATE EXTENSION", "DROP ", "GRANT ",
		"REVOKE ", "TRUNCATE ", "\\COPY", "\\INCLUDE", "\\IR",
	} {
		if strings.Contains(upper, forbidden) {
			return fmt.Errorf("PostgreSQL migration contains forbidden operation %s", forbidden)
		}
	}
	if err := validatePostgreSQLMigrationQuotes(trimmed); err != nil {
		return err
	}
	return nil
}

func validatePostgreSQLMigrationQuotes(source string) error {
	inSingleQuote := false
	dollarTag := ""
	for index := 0; index < len(source); {
		if dollarTag != "" {
			if strings.HasPrefix(source[index:], dollarTag) {
				index += len(dollarTag)
				dollarTag = ""
				continue
			}
			index++
			continue
		}
		if inSingleQuote {
			if source[index] == '\'' {
				if index+1 < len(source) && source[index+1] == '\'' {
					index += 2
					continue
				}
				inSingleQuote = false
			}
			index++
			continue
		}
		if source[index] == '\'' {
			inSingleQuote = true
			index++
			continue
		}
		if source[index] == '$' {
			end := strings.IndexByte(source[index+1:], '$')
			if end >= 0 {
				candidate := source[index : index+end+2]
				valid := true
				for _, value := range candidate[1 : len(candidate)-1] {
					if value != '_' && (value < 'A' || value > 'Z') &&
						(value < 'a' || value > 'z') && (value < '0' || value > '9') {
						valid = false
					}
				}
				if valid {
					dollarTag = candidate
					index += len(candidate)
					continue
				}
			}
		}
		index++
	}
	if inSingleQuote || dollarTag != "" {
		return fmt.Errorf("PostgreSQL migration contains an unterminated quoted value")
	}
	return nil
}
