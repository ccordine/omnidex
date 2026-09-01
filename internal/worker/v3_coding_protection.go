package worker

func directCodingProtectedPathSet(paths []string) map[string]struct{} {
	protected := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		protected[path] = struct{}{}
	}
	return protected
}

func directCodingPathProtected(path string, protected map[string]struct{}) bool {
	for protectedPath := range protected {
		if directCodingTargetTreeFileHierarchyConflict(path, protectedPath) {
			return true
		}
	}
	return false
}
