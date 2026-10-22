package main

import (
	"fmt"

	"github.com/gryph/omnidex/internal/sessionintent"
)

// The editor already displayed this submission. Match the server's committed
// identities, never text alone: another submission can contain the same words.
func (session *chatSession) rememberUserIntentEcho(receipt sessionintent.Receipt, text string) {
	if !session.renderer.console.IsTerminal() {
		return
	}
	if session.localEchoes == nil {
		session.localEchoes = make(map[string]string)
	}
	if receipt.MessageID > 0 {
		session.localEchoes[fmt.Sprintf("message:%020d", receipt.MessageID)] = text
	}
	if receipt.TurnOperationID != "" {
		session.localEchoes["turn:"+string(receipt.TurnOperationID)] = text
	}
	if receipt.ControlOperationID != "" {
		session.localEchoes["control:"+string(receipt.ControlOperationID)] = text
	}
}

func (session *chatSession) consumeLocalEcho(key, text string) bool {
	local, exists := session.localEchoes[key]
	delete(session.localEchoes, key)
	return exists && local == text
}
