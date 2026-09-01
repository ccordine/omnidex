package api

func scrumCardChannelChanged(before, after ScrumCard) bool {
	if len(before.PendingChannelMessages) != len(after.PendingChannelMessages) {
		return true
	}
	if len(before.PendingChannelMessages) == 0 {
		return false
	}
	lastBefore := before.PendingChannelMessages[len(before.PendingChannelMessages)-1]
	lastAfter := after.PendingChannelMessages[len(after.PendingChannelMessages)-1]
	return lastBefore.Content != lastAfter.Content || lastBefore.Role != lastAfter.Role
}
