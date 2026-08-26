package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingServiceStorageKind string

const (
	directCodingServiceStorageRequestLocal directCodingServiceStorageKind = "request_local"
	directCodingServiceStoragePostgreSQL   directCodingServiceStorageKind = "postgresql_json"

	directCodingServiceStateSchemaVersion = 1
	directCodingServiceStateSchemaTable   = "service_state_schema"
	directCodingServiceStateRecordTable   = "service_state_records"
	directCodingServiceStateDefaultKey    = "application"
)

// directCodingServiceStoragePlan is derived entirely from the accepted
// state-lifetime leaves. Models decide only lifetime semantics; code owns the
// concrete storage mechanism, schema, namespace, and task bindings.
type directCodingServiceStoragePlan struct {
	WorkloadSHA256 string
	Namespace      string
	ByTask         map[string]directCodingServiceStorageKind
}

func deriveDirectCodingServiceStoragePlan(
	workload assemblyline.FrozenApplicationWorkload,
	lifetimes directCodingServiceStatePlan,
) (directCodingServiceStoragePlan, error) {
	if err := lifetimes.ValidateFor(workload); err != nil {
		return directCodingServiceStoragePlan{}, err
	}
	plan := directCodingServiceStoragePlan{
		WorkloadSHA256: workload.SHA256,
		ByTask:         make(map[string]directCodingServiceStorageKind, len(workload.Tasks)),
	}
	for _, task := range workload.Tasks {
		switch lifetimes.ByTask[task.ID] {
		case assemblyline.ApplicationServiceStateRequestLocalOnly:
			plan.ByTask[task.ID] = directCodingServiceStorageRequestLocal
		case assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired:
			plan.ByTask[task.ID] = directCodingServiceStoragePostgreSQL
			plan.Namespace = "workload:" + workload.SHA256
		default:
			return directCodingServiceStoragePlan{}, fmt.Errorf(
				"service storage task %s has unsupported lifetime %q",
				task.ID, lifetimes.ByTask[task.ID],
			)
		}
	}
	return plan, nil
}

func (plan directCodingServiceStoragePlan) RequiresPostgreSQL() bool {
	for _, storage := range plan.ByTask {
		if storage == directCodingServiceStoragePostgreSQL {
			return true
		}
	}
	return false
}

func (plan directCodingServiceStoragePlan) storageForTask(
	taskID string,
) (directCodingServiceStorageKind, error) {
	storage, exists := plan.ByTask[taskID]
	if !exists {
		return "", fmt.Errorf("service storage plan omits task %s", taskID)
	}
	switch storage {
	case directCodingServiceStorageRequestLocal, directCodingServiceStoragePostgreSQL:
		return storage, nil
	default:
		return "", fmt.Errorf(
			"service storage task %s has unsupported mechanism %q", taskID, storage,
		)
	}
}

func (plan directCodingServiceStoragePlan) RequiresPostgreSQLForTask(
	taskID string,
) (bool, error) {
	storage, err := plan.storageForTask(taskID)
	if err != nil {
		return false, err
	}
	return storage == directCodingServiceStoragePostgreSQL, nil
}
