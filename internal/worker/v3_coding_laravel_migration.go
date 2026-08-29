package worker

import (
	"fmt"
	"strings"
)

const laravelStateMigrationPath = "database/migrations/0001_01_01_000000_create_service_state_tables.php"

func laravelServiceStateMigration(storage directCodingServiceStoragePlan) (string, error) {
	if storage.WorkloadSHA256 == "" || len(storage.ByTask) == 0 {
		return "", fmt.Errorf("Laravel migration requires one derived service storage plan")
	}
	if storage.RequiresPostgreSQL() && storage.Namespace == "" {
		return "", fmt.Errorf("Laravel durable storage plan lacks its workload namespace")
	}
	if !storage.RequiresPostgreSQL() {
		return "", fmt.Errorf("Laravel state migration requires durable PostgreSQL authority")
	}
	return laravelServiceStateMigrationSource(), nil
}

func laravelServiceStateMigrationSource() string {
	return fmt.Sprintf(`<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

return new class extends Migration
{
    public $withinTransaction = true;

    public function up(): void
    {
        DB::unprepared(<<<'SQL'
%s
SQL);
    }

    public function down(): void
    {
        DB::statement('DROP TABLE IF EXISTS %s');
        DB::statement('DROP TABLE IF EXISTS %s');
    }
};
`, strings.TrimSuffix(directCodingServiceStateSchemaStatements(), "\n"),
		directCodingServiceStateRecordTable, directCodingServiceStateSchemaTable)
}
