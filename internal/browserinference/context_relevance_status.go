package browserinference

// Ready reports only whether one browser session has completed model loading
// and connected to this broker. It grants no workflow or station authority.
func (broker *ContextRelevanceBroker) Ready() bool {
	if broker == nil {
		return false
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	return broker.active != nil
}
