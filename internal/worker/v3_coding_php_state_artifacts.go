package worker

import "fmt"

func directCodingServiceStateSchemaStatements() string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    version INTEGER NOT NULL CHECK (version > 0)
);
INSERT INTO %s (singleton, version)
VALUES (TRUE, %d)
ON CONFLICT (singleton) DO NOTHING;
DO $migration$
BEGIN
    IF (SELECT version FROM %s WHERE singleton = TRUE) IS DISTINCT FROM %d THEN
        RAISE EXCEPTION 'unsupported durable service-state schema version';
    END IF;
END
$migration$;
CREATE TABLE IF NOT EXISTS %s (
    state_scope TEXT NOT NULL CHECK (char_length(state_scope) BETWEEN 1 AND 200),
    state_key TEXT NOT NULL CHECK (char_length(state_key) BETWEEN 1 AND 200),
    state_value JSONB NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (state_scope, state_key)
);
`, directCodingServiceStateSchemaTable, directCodingServiceStateSchemaTable,
		directCodingServiceStateSchemaVersion, directCodingServiceStateSchemaTable,
		directCodingServiceStateSchemaVersion, directCodingServiceStateRecordTable)
}

func phpServiceStateMigrationSQL() string {
	return "BEGIN;\n" + directCodingServiceStateSchemaStatements() + "COMMIT;\n"
}

func phpServiceStateMigrationRunnerSource() string {
	return `<?php
declare(strict_types=1);

require_once __DIR__ . '/../src/Runtime.php';

$migration = file_get_contents(__DIR__ . '/migrations/001_service_state.sql');
if (!is_string($migration) || trim($migration) === '') {
    throw new RuntimeException('Durable service-state migration source is unavailable.');
}
RuntimeDatabase::connection(false)->exec($migration);
RuntimeDatabase::assertSchema();
`
}

func phpServiceStateVerifierSource(namespace string) string {
	return fmt.Sprintf(`<?php
declare(strict_types=1);

require_once __DIR__ . '/../src/Runtime.php';

$mode = $argv[1] ?? '';
$applicationScope = %s;
$verificationScope = %s;
$applicationKey = %s;
$verificationKey = '__omnidex_storage_verification__';
$expected = ['storage' => 'postgresql', 'schema_version' => %d];
$shape = [
    'records' => ['kind' => 'record_list', 'fields' => ['label' => 'string', 'rank' => 'integer']],
    'enabled' => ['kind' => 'boolean', 'fields' => []],
];
RuntimeState::assertShape([], $shape);
$invalidShapes = [
    ['unknown' => true],
    ['records' => [['unknown' => 'value']]],
    ['records' => [['rank' => 'wrong-kind']]],
    ['enabled' => 'wrong-kind'],
];
foreach ($invalidShapes as $invalidShape) {
    $rejected = false;
    try {
        RuntimeState::assertShape($invalidShape, $shape);
    } catch (InvalidArgumentException $failure) {
        $rejected = true;
    }
    if (!$rejected) {
        throw new RuntimeException('Durable state interface accepted an invalid shape.');
    }
}
if ($mode === 'write') {
    if (RuntimeState::load($verificationScope, $verificationKey) !== null) {
        throw new RuntimeException('Durable service-state verification found stale state.');
    }
    RuntimeState::save($verificationScope, $verificationKey, $expected);
    exit(0);
}
if ($mode === 'read') {
    if (RuntimeState::load($verificationScope, $verificationKey) !== $expected) {
        throw new RuntimeException('Durable service state did not survive a separate process.');
    }
    if (!RuntimeState::delete($verificationScope, $verificationKey) ||
        RuntimeState::load($verificationScope, $verificationKey) !== null) {
        throw new RuntimeException('Durable service-state delete was not authoritative.');
    }
    exit(0);
}
if ($mode === 'reset') {
    RuntimeState::delete($applicationScope, $applicationKey);
    if (RuntimeState::load($applicationScope, $applicationKey) !== null) {
        throw new RuntimeException('Durable application state reset was not authoritative.');
    }
    exit(0);
}
throw new InvalidArgumentException('StateVerifier requires exactly write, read, or reset mode.');
`, phpSingleQuoted(namespace), phpSingleQuoted(namespace+":verification"),
		phpSingleQuoted(directCodingServiceStateDefaultKey), directCodingServiceStateSchemaVersion)
}

func phpServiceStateVerificationEnvironment() string {
	return "SERVICE_STATE_DB_PASSWORD=" + phpServiceStateVerificationSecret + "\n"
}

func phpServiceStateDeploymentEnvironment() string {
	return "# Required: set one non-empty URI-safe deployment secret.\nSERVICE_STATE_DB_PASSWORD=\n"
}
