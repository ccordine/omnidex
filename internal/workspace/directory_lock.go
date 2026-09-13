package workspace

// A directory lock retains the platform-specific ownership needed to release
// the exact acquired authority. No lock file or workspace metadata is created.
type directoryLock interface{ Release() error }
