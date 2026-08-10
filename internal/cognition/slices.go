package cognition

// cloneSlice preserves the caller's nil-versus-explicit-empty authority while
// still returning independent storage. Several cognition values are hashed or
// journaled as JSON, where null and [] are different immutable payloads.
func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append([]T{}, values...)
}
