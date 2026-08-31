package worker

// RuntimeEvent is a bounded observation of work already owned by one claimed
// server job step. It carries no workflow or mutation authority.
type RuntimeEvent struct {
	JobID         int64
	StepID        int64
	Attempt       int64
	Kind          string
	Detail        string
	FileOperation string
	FilePath      string
}

// RuntimeEventSink projects worker observations into the server-owned
// transport. Returning an error keeps transport failures visible in worker
// logs without transferring workflow authority to the observer.
type RuntimeEventSink func(RuntimeEvent) error
