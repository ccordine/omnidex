package omni

import (
	"strings"
	"testing"
)

func TestComposePostgresStorageIsEphemeral(t *testing.T) {
	t.Parallel()
	root := repoRootFromOmniTest(t)
	compose := readRepoScript(t, root, "docker-compose.yml")
	postgresStart := strings.Index(compose, "\n  postgres:\n")
	redisStart := strings.Index(compose, "\n  redis:\n")
	if postgresStart < 0 || redisStart <= postgresStart {
		t.Fatal("Compose database service boundaries are unavailable")
	}
	postgres := compose[postgresStart:redisStart]
	if !strings.Contains(postgres, "\n    tmpfs:\n      - /var/lib/postgresql/data\n") {
		t.Fatal("Compose PostgreSQL data must use its explicit ephemeral tmpfs")
	}
	if strings.Contains(postgres, "\n    volumes:\n") ||
		strings.Contains(compose, "\n  pgdata:\n") {
		t.Fatal("Compose retains durable PostgreSQL storage")
	}
}
