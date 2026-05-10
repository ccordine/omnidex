# OmnidexNeo Contracts
## Priority-1 Specialist Role Contracts

Prepared: 2026-05-07 (US/Eastern)  
Contract Version: 1.0

## 1) Scope
This document defines strict machine-validated contracts for these roles:
- `migration_specialist`
- `schema_governor`
- `memory_curator`
- `retrieval_librarian`
- `security_specialist`

These contracts are designed for deterministic orchestration. Any output that fails schema or deterministic rule checks is rejected.

## 2) Global Contract Rules
1. All payloads are JSON objects.
2. All timestamps are RFC3339 UTC (example: `2026-05-07T05:42:10Z`).
3. All IDs are lowercase snake/kebab style and stable within a run.
4. All schemas default to `additionalProperties: false`.
5. Any missing required field is a hard parse failure.
6. Any enum mismatch is a hard parse failure.
7. All list orderings must be deterministic and stable.
8. Free-form prose is not accepted where structured fields are required.

## 3) Shared Envelopes

### 3.1 Request envelope
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "contract_version",
    "role_id",
    "run_id",
    "step_id",
    "permission_mode",
    "deadline_ms",
    "objective",
    "payload"
  ],
  "properties": {
    "contract_version": {"type": "string", "const": "1.0"},
    "role_id": {"type": "string"},
    "run_id": {"type": "string", "pattern": "^[a-z0-9][a-z0-9_-]{5,63}$"},
    "step_id": {"type": "string", "pattern": "^[a-z0-9][a-z0-9_-]{5,63}$"},
    "permission_mode": {"type": "string", "enum": ["ask_permission", "full_access"]},
    "deadline_ms": {"type": "integer", "minimum": 1000, "maximum": 3600000},
    "objective": {"type": "string", "minLength": 1, "maxLength": 12000},
    "input_artifact_ids": {
      "type": "array",
      "items": {"type": "string", "pattern": "^[a-z0-9][a-z0-9._:-]{3,127}$"},
      "uniqueItems": true
    },
    "payload": {"type": "object"}
  }
}
```

### 3.2 Response envelope
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "contract_version",
    "role_id",
    "run_id",
    "step_id",
    "status",
    "payload"
  ],
  "properties": {
    "contract_version": {"type": "string", "const": "1.0"},
    "role_id": {"type": "string"},
    "run_id": {"type": "string", "pattern": "^[a-z0-9][a-z0-9_-]{5,63}$"},
    "step_id": {"type": "string", "pattern": "^[a-z0-9][a-z0-9_-]{5,63}$"},
    "status": {"type": "string", "enum": ["success", "partial", "fail", "blocked"]},
    "payload": {"type": "object"},
    "error": {
      "type": "object",
      "additionalProperties": false,
      "required": ["code", "message", "retryable"],
      "properties": {
        "code": {"type": "string", "pattern": "^[a-z0-9_.-]{3,80}$"},
        "message": {"type": "string", "minLength": 1, "maxLength": 2000},
        "retryable": {"type": "boolean"}
      }
    },
    "produced_artifact_ids": {
      "type": "array",
      "items": {"type": "string", "pattern": "^[a-z0-9][a-z0-9._:-]{3,127}$"},
      "uniqueItems": true
    }
  }
}
```

### 3.3 Common error codes
- `parse_error.schema_invalid`
- `parse_error.required_field_missing`
- `policy_violation.forbidden_operation`
- `policy_violation.out_of_scope`
- `verification_failed.contract_rule`
- `tool_unavailable.required_context_missing`

## 4) Role: `migration_specialist`

### 4.1 Purpose
Produce one deterministic, reviewable migration bundle (Laravel-style up/down discipline) from a bounded change request.

### 4.2 Allowed tools
- `schema_snapshot_reader`
- `sql_lint`
- `policy_checker_sql`

### 4.3 Input payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "db_engine",
    "db_version_major",
    "migration_namespace",
    "change_requests",
    "existing_migration_ids",
    "constraints"
  ],
  "properties": {
    "db_engine": {"type": "string", "const": "postgresql"},
    "db_version_major": {"type": "integer", "minimum": 13, "maximum": 18},
    "migration_namespace": {"type": "string", "pattern": "^[a-z0-9_]{3,64}$"},
    "change_requests": {
      "type": "array",
      "minItems": 1,
      "maxItems": 20,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["change_id", "kind", "description"],
        "properties": {
          "change_id": {"type": "string", "pattern": "^[a-z0-9_-]{3,64}$"},
          "kind": {
            "type": "string",
            "enum": [
              "create_table",
              "alter_table",
              "drop_table",
              "create_index",
              "drop_index",
              "create_extension",
              "alter_type",
              "raw_sql"
            ]
          },
          "description": {"type": "string", "minLength": 1, "maxLength": 2000}
        }
      }
    },
    "existing_migration_ids": {
      "type": "array",
      "items": {"type": "string", "pattern": "^[0-9]{14}_[a-z0-9_]{3,120}$"},
      "uniqueItems": true
    },
    "constraints": {
      "type": "object",
      "additionalProperties": false,
      "required": ["require_down_sql", "allow_out_of_txn", "max_lock_risk"],
      "properties": {
        "require_down_sql": {"type": "boolean"},
        "allow_out_of_txn": {"type": "boolean"},
        "max_lock_risk": {"type": "string", "enum": ["low", "medium", "high"]}
      }
    }
  }
}
```

### 4.4 Output payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "migration_id",
    "file_path",
    "transaction_mode",
    "steps",
    "preflight_sql",
    "postflight_sql"
  ],
  "properties": {
    "migration_id": {"type": "string", "pattern": "^[0-9]{14}_[a-z0-9_]{3,120}$"},
    "file_path": {"type": "string", "pattern": "^database/migrations/[0-9]{14}_[a-z0-9_]{3,120}\\.sql$"},
    "transaction_mode": {"type": "string", "enum": ["single_transaction", "mixed", "out_of_txn"]},
    "steps": {
      "type": "array",
      "minItems": 1,
      "maxItems": 100,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "step_no",
          "up_sql",
          "down_sql",
          "requires_out_of_txn",
          "lock_risk"
        ],
        "properties": {
          "step_no": {"type": "integer", "minimum": 1, "maximum": 1000},
          "up_sql": {"type": "string", "minLength": 1, "maxLength": 20000},
          "down_sql": {"type": "string", "minLength": 1, "maxLength": 20000},
          "requires_out_of_txn": {"type": "boolean"},
          "lock_risk": {"type": "string", "enum": ["low", "medium", "high"]}
        }
      }
    },
    "preflight_sql": {
      "type": "array",
      "items": {"type": "string", "minLength": 1, "maxLength": 4000},
      "maxItems": 20
    },
    "postflight_sql": {
      "type": "array",
      "items": {"type": "string", "minLength": 1, "maxLength": 4000},
      "maxItems": 20
    },
    "warnings": {
      "type": "array",
      "items": {"type": "string", "minLength": 1, "maxLength": 500}
    }
  }
}
```

### 4.5 Deterministic validation rules
1. Exactly one migration bundle per response.
2. `steps` must be strictly increasing by `step_no` and contiguous from 1.
3. Every `up_sql` must have paired `down_sql`.
4. If SQL includes any of the following, `requires_out_of_txn` must be `true` for that step:
- `CREATE INDEX CONCURRENTLY`
- `DROP INDEX CONCURRENTLY`
- `CREATE DATABASE`
- `VACUUM`
- `ALTER SYSTEM`
- `CREATE TABLESPACE`
5. If any step has `requires_out_of_txn=true`, `transaction_mode` must be `mixed` or `out_of_txn`.
6. Forbidden SQL patterns (hard block):
- `COPY ... PROGRAM`
- `ALTER SYSTEM`
- `DO $$` with dynamic `EXECUTE` unless explicitly allowlisted by policy snapshot.
7. `preflight_sql` and `postflight_sql` must be read-only statements.

### 4.6 Example output
```json
{
  "contract_version": "1.0",
  "role_id": "migration_specialist",
  "run_id": "run_ab12cd34ef56",
  "step_id": "step_001_schema",
  "status": "success",
  "payload": {
    "migration_id": "20260507061500_create_runs_table",
    "file_path": "database/migrations/20260507061500_create_runs_table.sql",
    "transaction_mode": "single_transaction",
    "steps": [
      {
        "step_no": 1,
        "up_sql": "CREATE TABLE omnidex.runs (id BIGSERIAL PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT now());",
        "down_sql": "DROP TABLE IF EXISTS omnidex.runs;",
        "requires_out_of_txn": false,
        "lock_risk": "low"
      }
    ],
    "preflight_sql": [
      "SELECT to_regclass('omnidex.runs') IS NULL AS ok;"
    ],
    "postflight_sql": [
      "SELECT to_regclass('omnidex.runs') IS NOT NULL AS ok;"
    ]
  }
}
```

## 5) Role: `schema_governor`

### 5.1 Purpose
Gate migration safety and order, detect drift, and produce deterministic allow/block decision.

### 5.2 Allowed tools
- `migration_ledger_reader`
- `checksum_verifier`
- `sql_policy_checker`

### 5.3 Input payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "proposed_migration",
    "applied_migration_ledger",
    "policy"
  ],
  "properties": {
    "proposed_migration": {"type": "object"},
    "applied_migration_ledger": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["migration_id", "batch", "checksum"],
        "properties": {
          "migration_id": {"type": "string", "pattern": "^[0-9]{14}_[a-z0-9_]{3,120}$"},
          "batch": {"type": "integer", "minimum": 1},
          "checksum": {"type": "string", "pattern": "^[a-f0-9]{64}$"}
        }
      }
    },
    "policy": {
      "type": "object",
      "additionalProperties": false,
      "required": ["require_checksum_match", "require_monotonic_order"],
      "properties": {
        "require_checksum_match": {"type": "boolean"},
        "require_monotonic_order": {"type": "boolean"}
      }
    }
  }
}
```

### 5.4 Output payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["decision", "violations", "checks"],
  "properties": {
    "decision": {"type": "string", "enum": ["approve", "reject", "needs_human"]},
    "violations": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["code", "severity", "message"],
        "properties": {
          "code": {"type": "string", "pattern": "^[a-z0-9_.-]{3,80}$"},
          "severity": {"type": "string", "enum": ["low", "medium", "high", "critical"]},
          "message": {"type": "string", "minLength": 1, "maxLength": 1000}
        }
      }
    },
    "checks": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["check_id", "passed"],
        "properties": {
          "check_id": {"type": "string", "pattern": "^[a-z0-9_.-]{3,80}$"},
          "passed": {"type": "boolean"},
          "details": {"type": "string", "maxLength": 1000}
        }
      }
    }
  }
}
```

### 5.5 Deterministic validation rules
1. Reject if proposed `migration_id` already exists in applied ledger.
2. Reject on checksum drift for existing `migration_id`.
3. Reject if migration timestamp order regresses against ledger max ID.
4. Reject if `down_sql` is missing while policy requires rollback support.
5. `decision=approve` requires zero `high` or `critical` violations.

## 6) Role: `memory_curator`

### 6.1 Purpose
Transform raw memory candidates into approved durable memory entries with provenance and deduplication.

### 6.2 Allowed tools
- `artifact_reader`
- `embedding_lookup`
- `memory_ledger_reader`

### 6.3 Input payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["candidate_memories", "dedupe_threshold", "policy"],
  "properties": {
    "candidate_memories": {
      "type": "array",
      "minItems": 1,
      "maxItems": 200,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["candidate_id", "memory_text", "provenance"],
        "properties": {
          "candidate_id": {"type": "string", "pattern": "^[a-z0-9_-]{3,80}$"},
          "memory_text": {"type": "string", "minLength": 10, "maxLength": 4000},
          "tags": {
            "type": "array",
            "items": {"type": "string", "pattern": "^[a-z0-9_:-]{2,40}$"},
            "maxItems": 20,
            "uniqueItems": true
          },
          "provenance": {
            "type": "array",
            "minItems": 1,
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": ["artifact_id", "offset_start", "offset_end"],
              "properties": {
                "artifact_id": {"type": "string", "pattern": "^[a-z0-9][a-z0-9._:-]{3,127}$"},
                "offset_start": {"type": "integer", "minimum": 0},
                "offset_end": {"type": "integer", "minimum": 0}
              }
            }
          }
        }
      }
    },
    "dedupe_threshold": {"type": "number", "minimum": 0, "maximum": 1},
    "policy": {
      "type": "object",
      "additionalProperties": false,
      "required": ["require_provenance", "max_text_length"],
      "properties": {
        "require_provenance": {"type": "boolean"},
        "max_text_length": {"type": "integer", "minimum": 64, "maximum": 8000}
      }
    }
  }
}
```

### 6.4 Output payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["accepted", "rejected", "merged_clusters"],
  "properties": {
    "accepted": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["candidate_id", "memory_entry_key", "canonical_text"],
        "properties": {
          "candidate_id": {"type": "string", "pattern": "^[a-z0-9_-]{3,80}$"},
          "memory_entry_key": {"type": "string", "pattern": "^[a-z0-9_:-]{4,120}$"},
          "canonical_text": {"type": "string", "minLength": 10, "maxLength": 4000}
        }
      }
    },
    "rejected": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["candidate_id", "reason_code"],
        "properties": {
          "candidate_id": {"type": "string", "pattern": "^[a-z0-9_-]{3,80}$"},
          "reason_code": {
            "type": "string",
            "enum": [
              "missing_provenance",
              "duplicate_existing",
              "too_vague",
              "policy_blocked",
              "unsupported_format"
            ]
          },
          "message": {"type": "string", "maxLength": 500}
        }
      }
    },
    "merged_clusters": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["cluster_id", "candidate_ids"],
        "properties": {
          "cluster_id": {"type": "string", "pattern": "^[a-z0-9_-]{3,80}$"},
          "candidate_ids": {
            "type": "array",
            "minItems": 2,
            "items": {"type": "string", "pattern": "^[a-z0-9_-]{3,80}$"},
            "uniqueItems": true
          }
        }
      }
    }
  }
}
```

### 6.5 Deterministic validation rules
1. `accepted` and `rejected` sets must be disjoint by `candidate_id`.
2. Any candidate without provenance is rejected if `require_provenance=true`.
3. `canonical_text` must be deterministic normalization of source text (trimmed, whitespace-normalized).
4. `merged_clusters` can only reference candidates present in input.

## 7) Role: `retrieval_librarian`

### 7.1 Purpose
Produce deterministic retrieval plans (lexical/vector/hybrid) with explicit budgets and rerank policy.

### 7.2 Allowed tools
- `artifact_index_lookup`
- `pgvector_search`
- `keyword_search`

### 7.3 Input payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["query", "allowed_sources", "budget"],
  "properties": {
    "query": {"type": "string", "minLength": 3, "maxLength": 2000},
    "allowed_sources": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "string",
        "enum": ["run_artifacts", "memory_entries", "memory_candidates", "web_cache"]
      },
      "uniqueItems": true
    },
    "budget": {
      "type": "object",
      "additionalProperties": false,
      "required": ["max_candidates", "max_context_tokens"],
      "properties": {
        "max_candidates": {"type": "integer", "minimum": 1, "maximum": 200},
        "max_context_tokens": {"type": "integer", "minimum": 128, "maximum": 16000}
      }
    },
    "filters": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "min_recency_ts": {"type": "string", "format": "date-time"},
        "tags_any": {
          "type": "array",
          "items": {"type": "string", "pattern": "^[a-z0-9_:-]{2,40}$"},
          "uniqueItems": true,
          "maxItems": 20
        }
      }
    }
  }
}
```

### 7.4 Output payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["strategy", "plan_steps", "rerank_policy", "expected_output"],
  "properties": {
    "strategy": {"type": "string", "enum": ["lexical_only", "vector_only", "hybrid"]},
    "plan_steps": {
      "type": "array",
      "minItems": 1,
      "maxItems": 20,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["step_no", "op", "limit"],
        "properties": {
          "step_no": {"type": "integer", "minimum": 1, "maximum": 100},
          "op": {
            "type": "string",
            "enum": ["keyword_search", "vector_search", "merge", "rerank", "truncate_context"]
          },
          "limit": {"type": "integer", "minimum": 1, "maximum": 500},
          "notes": {"type": "string", "maxLength": 500}
        }
      }
    },
    "rerank_policy": {
      "type": "object",
      "additionalProperties": false,
      "required": ["method", "weights"],
      "properties": {
        "method": {"type": "string", "enum": ["weighted_sum"]},
        "weights": {
          "type": "object",
          "additionalProperties": false,
          "required": ["semantic", "lexical", "recency"],
          "properties": {
            "semantic": {"type": "number", "minimum": 0, "maximum": 1},
            "lexical": {"type": "number", "minimum": 0, "maximum": 1},
            "recency": {"type": "number", "minimum": 0, "maximum": 1}
          }
        }
      }
    },
    "expected_output": {
      "type": "object",
      "additionalProperties": false,
      "required": ["max_context_tokens", "target_candidates"],
      "properties": {
        "max_context_tokens": {"type": "integer", "minimum": 128, "maximum": 16000},
        "target_candidates": {"type": "integer", "minimum": 1, "maximum": 200}
      }
    }
  }
}
```

### 7.5 Deterministic validation rules
1. `plan_steps` must be contiguous `step_no` sequence starting at 1.
2. `strategy=hybrid` requires at least one `keyword_search` and one `vector_search` step.
3. Rerank weights must sum to exactly `1.0` with tolerance `1e-6`.
4. `expected_output.max_context_tokens` cannot exceed input budget.

## 8) Role: `security_specialist`

### 8.1 Purpose
Classify action plans by risk, enforce policy gates, and return allow/block/require-permission verdicts.

### 8.2 Allowed tools
- `command_static_analyzer`
- `url_policy_checker`
- `secrets_detector`

### 8.3 Input payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["proposed_actions", "permission_mode", "policy_snapshot"],
  "properties": {
    "proposed_actions": {
      "type": "array",
      "minItems": 1,
      "maxItems": 200,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["action_id", "action_type", "content"],
        "properties": {
          "action_id": {"type": "string", "pattern": "^[a-z0-9_-]{3,80}$"},
          "action_type": {
            "type": "string",
            "enum": ["shell_command", "sql_statement", "http_request", "file_write", "tool_call"]
          },
          "content": {"type": "string", "minLength": 1, "maxLength": 12000}
        }
      }
    },
    "permission_mode": {"type": "string", "enum": ["ask_permission", "full_access"]},
    "policy_snapshot": {
      "type": "object",
      "additionalProperties": false,
      "required": ["deny_patterns", "allow_domains"],
      "properties": {
        "deny_patterns": {
          "type": "array",
          "items": {"type": "string", "minLength": 1, "maxLength": 200},
          "maxItems": 200
        },
        "allow_domains": {
          "type": "array",
          "items": {"type": "string", "minLength": 1, "maxLength": 255},
          "maxItems": 200,
          "uniqueItems": true
        }
      }
    }
  }
}
```

### 8.4 Output payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["overall_decision", "action_results"],
  "properties": {
    "overall_decision": {"type": "string", "enum": ["allow", "block", "require_permission"]},
    "action_results": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["action_id", "decision", "risk_score", "risk_tier"],
        "properties": {
          "action_id": {"type": "string", "pattern": "^[a-z0-9_-]{3,80}$"},
          "decision": {"type": "string", "enum": ["allow", "block", "require_permission"]},
          "risk_score": {"type": "integer", "minimum": 0, "maximum": 100},
          "risk_tier": {"type": "integer", "minimum": 0, "maximum": 3},
          "violations": {
            "type": "array",
            "items": {"type": "string", "pattern": "^[a-z0-9_.-]{3,80}$"},
            "uniqueItems": true,
            "maxItems": 50
          },
          "sanitized_alternative": {"type": "string", "maxLength": 12000}
        }
      }
    }
  }
}
```

### 8.5 Deterministic validation rules
1. Any action matching deny patterns must return `decision=block`.
2. In `ask_permission` mode, tiers 1-3 must produce `require_permission` unless blocked.
3. In `full_access` mode, tier 1-3 may be `allow` if not blocked by policy.
4. `overall_decision` is computed as:
- `block` if any action is blocked.
- otherwise `require_permission` if any action requires permission.
- otherwise `allow`.

## 9) Role-Specific Failure Codes

### 9.1 `migration_specialist`
- `verification_failed.missing_down_sql`
- `verification_failed.non_deterministic_steps`
- `policy_violation.forbidden_sql_pattern`
- `policy_violation.out_of_txn_not_allowed`

### 9.2 `schema_governor`
- `verification_failed.migration_id_collision`
- `verification_failed.ledger_checksum_drift`
- `verification_failed.order_regression`

### 9.3 `memory_curator`
- `verification_failed.provenance_missing`
- `verification_failed.dedupe_conflict`

### 9.4 `retrieval_librarian`
- `verification_failed.weight_sum_invalid`
- `verification_failed.plan_budget_exceeded`

### 9.5 `security_specialist`
- `policy_violation.deny_pattern_match`
- `policy_violation.domain_not_allowlisted`
- `policy_violation.secret_exposure_risk`

## 10) Contract Test Vectors (Minimum Required)
For each role, maintain fixtures with:
1. one valid payload
2. one schema-invalid payload
3. one policy-block payload
4. one edge-case payload at limits

Store fixtures under:
- `tests/contracts/<role_id>/valid/*.json`
- `tests/contracts/<role_id>/invalid/*.json`

## 11) Versioning Rules
1. Contract changes are backward incompatible unless explicitly marked additive.
2. Any required-field addition bumps major contract version.
3. Enum expansion bumps minor contract version.
4. Deprecated fields remain accepted for one full release cycle, then removed.

## 12) System Contract: Intent Gate (pre-router)

### 12.1 Purpose
Classify each user turn as conversation vs execution before any tool routing.

### 12.2 Input payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["turn_id", "message_text", "workspace_path", "permission_mode"],
  "properties": {
    "turn_id": {"type": "string", "pattern": "^[a-z0-9_-]{3,80}$"},
    "message_text": {"type": "string", "minLength": 1, "maxLength": 12000},
    "workspace_path": {"type": "string", "minLength": 1, "maxLength": 1024},
    "permission_mode": {"type": "string", "enum": ["ask_permission", "full_access"]},
    "recent_turn_summaries": {
      "type": "array",
      "items": {"type": "string", "maxLength": 1000},
      "maxItems": 20
    }
  }
}
```

### 12.3 Output payload schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["intent_classification", "confidence", "reason_codes"],
  "properties": {
    "intent_classification": {
      "type": "string",
      "enum": ["conversation_mode", "execution_mode", "ambiguous"]
    },
    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    "reason_codes": {
      "type": "array",
      "minItems": 1,
      "items": {"type": "string", "pattern": "^[a-z0-9_.-]{3,80}$"},
      "uniqueItems": true,
      "maxItems": 20
    },
    "requires_clarification": {"type": "boolean"}
  }
}
```

### 12.4 Deterministic rules
1. `conversation_mode` must not dispatch side-effecting tools.
2. `execution_mode` may route tools, subject to permission and policy gates.
3. `ambiguous` must set `requires_clarification=true` and block side effects.
4. If `confidence < 0.70`, classification must be `ambiguous`.

## 13) References
- JSON Schema draft 2020-12: https://json-schema.org/draft/2020-12/schema
- PostgreSQL CREATE INDEX (transaction behavior for CONCURRENTLY): https://www.postgresql.org/docs/15/sql-createindex.html
- PostgreSQL DROP INDEX (transaction behavior for CONCURRENTLY): https://www.postgresql.org/docs/current/sql-dropindex.html
- PostgreSQL CREATE DATABASE: https://www.postgresql.org/docs/current/sql-createdatabase.html
- PostgreSQL VACUUM: https://www.postgresql.org/docs/current/sql-vacuum.html
- PostgreSQL ALTER SYSTEM: https://www.postgresql.org/docs/current/sql-altersystem.html
- PostgreSQL CREATE TABLESPACE: https://www.postgresql.org/docs/current/sql-createtablespace.html
- PostgreSQL COPY privileges and PROGRAM: https://www.postgresql.org/docs/current/sql-copy.html
