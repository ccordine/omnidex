package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentSettingsRequireExplicitServerAuthority(t *testing.T) {
	t.Parallel()
	valid := DeploymentSettings{
		KeyFile: "/var/lib/omnidex-deployment/key", BindAddress: "0.0.0.0",
		AdvertisedHost: "service.example.test", ProbeHost: "host.docker.internal",
	}
	if err := validateDirectCodingDeploymentSettings(valid); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*DeploymentSettings){
		"relative key":     func(value *DeploymentSettings) { value.KeyFile = "secret" },
		"invalid bind":     func(value *DeploymentSettings) { value.BindAddress = "localhost" },
		"specific bind":    func(value *DeploymentSettings) { value.BindAddress = "127.0.0.2" },
		"IPv6 bind":        func(value *DeploymentSettings) { value.BindAddress = "::" },
		"wildcard advert":  func(value *DeploymentSettings) { value.AdvertisedHost = "0.0.0.0" },
		"uppercase advert": func(value *DeploymentSettings) { value.AdvertisedHost = "Service.example.test" },
		"probe URL":        func(value *DeploymentSettings) { value.ProbeHost = "http://localhost" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := validateDirectCodingDeploymentSettings(candidate); err == nil {
				t.Fatal("accepted invalid deployment settings")
			}
		})
	}
}

func TestDeploymentKeyIsCreatedOnceAndRequiresExactMode(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "deployment.key")
	first, err := loadOrCreateDirectCodingDeploymentKey(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateDirectCodingDeploymentKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || len(first) != directCodingDeploymentKeyBytes {
		t.Fatal("deployment key was not stable")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateDirectCodingDeploymentKey(path); err == nil ||
		!strings.Contains(err.Error(), "mode 0600") {
		t.Fatalf("insecure key error=%v", err)
	}
}

func TestDeploymentSecretsAreDeterministicAndAuthorityBound(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789abcdef0123456789abcdef")
	first, err := deriveDirectCodingDeploymentSecrets(key, 41, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := deriveDirectCodingDeploymentSecrets(key, 41, 2)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := deriveDirectCodingDeploymentSecrets(key, 42, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first["APP_KEY"] != second["APP_KEY"] ||
		first["APP_KEY"] == changed["APP_KEY"] {
		t.Fatal("deployment secret derivation lost exact authority binding")
	}
	if !strings.HasPrefix(first["APP_KEY"], "base64:") ||
		first["DATABASE_PASSWORD"] != first["SERVICE_STATE_DB_PASSWORD"] {
		t.Fatalf("deployment secrets=%+v", first)
	}
}

func TestDeploymentSecretSetDigestCoversOnlyExactRequiredNames(t *testing.T) {
	t.Parallel()
	environment := map[string]string{
		"APP_KEY": "application-secret", "DATABASE_PASSWORD": "database-secret",
		"HOST_BIND_ADDRESS": "0.0.0.0", "HOST_HTTP_PORT": "49173",
	}
	first, err := directCodingDeploymentSecretSetSHA256(
		[]string{"APP_KEY", "DATABASE_PASSWORD"}, environment,
	)
	if err != nil {
		t.Fatal(err)
	}
	environment["HOST_HTTP_PORT"] = "49174"
	second, err := directCodingDeploymentSecretSetSHA256(
		[]string{"APP_KEY", "DATABASE_PASSWORD"}, environment,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatal("non-secret environment changed the exact secret-set digest")
	}
	environment["DATABASE_PASSWORD"] = "rotated-database-secret"
	third, err := directCodingDeploymentSecretSetSHA256(
		[]string{"APP_KEY", "DATABASE_PASSWORD"}, environment,
	)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("required secret mutation did not change the secret-set digest")
	}
	if _, err := directCodingDeploymentSecretSetSHA256(
		[]string{"DATABASE_PASSWORD", "APP_KEY"}, environment,
	); err == nil {
		t.Fatal("accepted unordered deployment secret names")
	}
}
