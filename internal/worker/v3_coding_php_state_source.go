package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	phpServiceStateMigrationPath      = "database/migrations/001_service_state.sql"
	phpServiceStateMigrationRunner    = "database/migrate.php"
	phpServiceStateVerificationPath   = "tests/StateVerifier.php"
	phpServiceStateVerificationEnv    = "tests/service-state.env.example"
	phpServiceStateDeploymentEnv      = ".env.example"
	phpServiceStateVerificationSecret = "omnidex-verification-only-password"
	phpServiceStateResetFunctionName  = "resetApplicationVerificationState"
)

func phpServiceStateRuntimeAPI() string {
	return `final class RuntimeState {
  public static function load(string $scope, string $key): ?array;
  public static function save(string $scope, string $key, array $value): void;
  public static function delete(string $scope, string $key): bool;
  public static function assertShape(array $value, array $schema): void;
}`
}

func phpServiceStateResetFunctionSource(namespace string) string {
	return fmt.Sprintf(`function %s(): void
{
    RuntimeState::delete(%s, %s);
    if (RuntimeState::load(%s, %s) !== null) {
        throw new RuntimeException('Durable application state reset was not authoritative.');
    }
}`, phpServiceStateResetFunctionName, phpSingleQuoted(namespace),
		phpSingleQuoted(directCodingServiceStateDefaultKey), phpSingleQuoted(namespace),
		phpSingleQuoted(directCodingServiceStateDefaultKey))
}

func phpServiceStateFacadeBlock(
	binding phpServiceFeatureBinding,
	namespace string,
	stateInterface directCodingServiceStateInterfaceBinding,
	writable bool,
) assemblyline.SourceBlock {
	return assemblyline.SourceBlock{
		ID: binding.StateBlockID,
		Static: phpServiceStateInterfaceFacadeSource(
			binding.StateClassName, namespace, stateInterface, writable,
		),
		API:       phpServiceStateInterfaceAPI(binding.StateClassName, stateInterface, writable),
		DependsOn: []string{"runtime.state"}, TaskID: binding.TaskID,
		Role: assemblyline.SourceBlockTaskSupport,
	}
}

func phpServiceStateRuntimeSource() string {
	return fmt.Sprintf(`final class RuntimeDatabase
{
    private static ?PDO $connection = null;
    private static bool $schemaVerified = false;

    public static function connection(bool $requireSchema = true): PDO
    {
        if (self::$connection === null) {
            if (!in_array('pgsql', PDO::getAvailableDrivers(), true)) {
                throw new RuntimeException('PostgreSQL PDO driver is unavailable.');
            }
            $url = getenv('DATABASE_URL');
            if (!is_string($url) || trim($url) === '') {
                throw new RuntimeException('DATABASE_URL is required for durable service state.');
            }
            $parts = parse_url($url);
            if (!is_array($parts) || ($parts['scheme'] ?? '') !== 'postgresql' ||
                isset($parts['query']) || isset($parts['fragment'])) {
                throw new RuntimeException('DATABASE_URL must be one PostgreSQL URL without query or fragment.');
            }
            foreach (['host', 'port', 'user', 'pass', 'path'] as $field) {
                if (!isset($parts[$field]) || trim((string) $parts[$field]) === '') {
                    throw new RuntimeException('DATABASE_URL is missing required ' . $field . '.');
                }
            }
            $host = (string) $parts['host'];
			$user = (string) $parts['user'];
			$password = (string) $parts['pass'];
            $port = filter_var($parts['port'], FILTER_VALIDATE_INT,
                ['options' => ['min_range' => 1, 'max_range' => 65535]]);
            $database = rawurldecode(ltrim((string) $parts['path'], '/'));
            if (preg_match('/^[A-Za-z0-9.-]+$/D', $host) !== 1 || $port === false ||
				preg_match('/^[A-Za-z0-9_]+$/D', $database) !== 1 ||
				preg_match('/^[A-Za-z0-9._~-]+$/D', $user) !== 1 ||
				preg_match('/^[A-Za-z0-9._~-]+$/D', $password) !== 1) {
				throw new RuntimeException('DATABASE_URL contains an invalid host, port, database, user, or URI-safe password.');
            }
            self::$connection = new PDO(
                'pgsql:host=' . $host . ';port=' . $port . ';dbname=' . $database,
				$user, $password,
                [PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION,
                 PDO::ATTR_EMULATE_PREPARES => false,
                 PDO::ATTR_STRINGIFY_FETCHES => false],
            );
        }
        if ($requireSchema) {
            self::assertSchema();
        }
        return self::$connection;
    }

    public static function assertSchema(): void
    {
        if (self::$schemaVerified) {
            return;
        }
        $statement = self::$connection?->query(
            'SELECT version FROM %s WHERE singleton = TRUE'
        );
        $version = $statement?->fetchColumn();
        if ($version === false || (int) $version !== %d) {
            throw new RuntimeException('Durable service-state schema version is missing or unsupported.');
        }
        self::$schemaVerified = true;
    }
}

final class RuntimeState
{
    private static array $revisions = [];

    public static function load(string $scope, string $key): ?array
    {
        self::validateKey('scope', $scope);
        self::validateKey('key', $key);
        $statement = RuntimeDatabase::connection()->prepare(
            'SELECT state_value, revision FROM %s WHERE state_scope = :scope AND state_key = :key'
        );
        $statement->execute(['scope' => $scope, 'key' => $key]);
        $row = $statement->fetch(PDO::FETCH_ASSOC);
        $identity = self::identity($scope, $key);
        if ($row === false) {
            self::$revisions[$identity] = 0;
            return null;
        }
        $revision = filter_var($row['revision'] ?? null, FILTER_VALIDATE_INT,
            ['options' => ['min_range' => 1]]);
        $value = json_decode((string) ($row['state_value'] ?? ''), true, 512, JSON_THROW_ON_ERROR);
        if ($revision === false || !is_array($value)) {
            throw new RuntimeException('Durable service state or revision is invalid.');
        }
        self::$revisions[$identity] = $revision;
        return $value;
    }

    public static function save(string $scope, string $key, array $value): void
    {
        self::validateKey('scope', $scope);
        self::validateKey('key', $key);
        $identity = self::identity($scope, $key);
        if (!array_key_exists($identity, self::$revisions)) {
            throw new RuntimeException('Durable service state must be loaded before it is saved.');
        }
        $revision = self::$revisions[$identity];
        $parameters = [
            'scope' => $scope, 'key' => $key,
            'value' => json_encode($value, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES),
        ];
        if ($revision === 0) {
            $statement = RuntimeDatabase::connection()->prepare(
                'INSERT INTO %s (state_scope, state_key, state_value) ' .
                'VALUES (:scope, :key, CAST(:value AS JSONB)) ON CONFLICT DO NOTHING'
            );
        } else {
            $statement = RuntimeDatabase::connection()->prepare(
                'UPDATE %s SET state_value = CAST(:value AS JSONB), revision = revision + 1, ' .
                'updated_at = NOW() WHERE state_scope = :scope AND state_key = :key AND revision = :revision'
            );
            $parameters['revision'] = $revision;
        }
        $statement->execute($parameters);
        if ($statement->rowCount() !== 1) {
            throw new RuntimeException('Durable service-state write lost optimistic revision authority.');
        }
        self::$revisions[$identity] = $revision + 1;
    }

    public static function delete(string $scope, string $key): bool
    {
        self::validateKey('scope', $scope);
        self::validateKey('key', $key);
        $statement = RuntimeDatabase::connection()->prepare(
            'DELETE FROM %s WHERE state_scope = :scope AND state_key = :key'
        );
        $statement->execute(['scope' => $scope, 'key' => $key]);
        if ($statement->rowCount() > 1) {
            throw new RuntimeException('Durable service-state delete changed an unexpected row count.');
        }
        unset(self::$revisions[self::identity($scope, $key)]);
        return $statement->rowCount() === 1;
    }

%s

    private static function identity(string $scope, string $key): string
    {
        return $scope . "\0" . $key;
    }

    private static function validateKey(string $label, string $value): void
    {
        if ($value === '' || strlen($value) > 200 || preg_match('/[\\x00-\\x1F\\x7F]/', $value) === 1) {
            throw new InvalidArgumentException('Durable service-state ' . $label . ' is invalid.');
        }
    }
}`, directCodingServiceStateSchemaTable, directCodingServiceStateSchemaVersion,
		directCodingServiceStateRecordTable, directCodingServiceStateRecordTable,
		directCodingServiceStateRecordTable, directCodingServiceStateRecordTable,
		phpServiceStateShapeMethodsSource())
}
