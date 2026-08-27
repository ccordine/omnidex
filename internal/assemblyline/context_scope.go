package assemblyline

import "fmt"

// ContextScope is code-owned transport metadata. It classifies the semantic
// domain whose authorities are being reduced without becoming model context.
type ContextScope string

const ContextScopeRoleplaySimulation ContextScope = "roleplay_simulation"

func (scope ContextScope) Validate() error {
	switch scope {
	case "", ContextScopeRoleplaySimulation:
		return nil
	default:
		return fmt.Errorf("context scope %q is not registered", scope)
	}
}
