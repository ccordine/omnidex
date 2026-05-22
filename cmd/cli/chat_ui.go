package main

import (
	"fmt"
	"os"
	"strings"
)

type chatUI struct {
	color bool
}

func newChatUI() *chatUI {
	return &chatUI{color: stdoutSupportsANSIColor()}
}

func stdoutSupportsANSIColor() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}

	if strings.TrimSpace(os.Getenv("CLICOLOR_FORCE")) == "1" {
		return true
	}

	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if term == "" || term == "dumb" {
		return false
	}

	if strings.TrimSpace(os.Getenv("CLICOLOR")) == "0" {
		return false
	}

	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func (ui *chatUI) paint(text, style string) string {
	if ui == nil || !ui.color || strings.TrimSpace(style) == "" {
		return text
	}
	return style + text + ansiReset
}

func (ui *chatUI) labelUser() string {
	return ui.paint("YOU>", ansiBold+ansiCyan)
}

func (ui *chatUI) labelAssistant() string {
	return ui.paint("AI >", ansiBold+ansiMagenta)
}

func (ui *chatUI) labelSystem() string {
	return ui.paint("SYS>", ansiBold+ansiBlue)
}

func (ui *chatUI) labelError() string {
	return ui.paint("ERR>", ansiBold+ansiRed)
}

func (ui *chatUI) labelNeedInput() string {
	return ui.paint("ASK>", ansiBold+ansiYellow)
}

func (ui *chatUI) labelFeedback() string {
	return ui.paint("FB >", ansiBold+ansiYellow)
}

func (ui *chatUI) promptUser() string {
	return ui.labelUser() + " "
}

func (ui *chatUI) promptFeedback() string {
	return ui.labelFeedback() + " "
}

func (ui *chatUI) rule() string {
	return ui.paint(strings.Repeat("-", 72), ansiDim)
}

func (ui *chatUI) printBanner(session string, architectMode bool) {
	fmt.Println(ui.rule())
	ui.printSystem("interactive chat started (session=" + session + ")")
	ui.printSystem("type /help for commands. type /exit to quit.")
	ui.printSystem(queuedTurnHintText())
	if architectMode {
		ui.printSystem("profile=architect enabled (workspace=on, reasoning=deep, verify=on, approval=on, verbose=on)")
	}
	fmt.Println(ui.rule())
}

func queuedTurnHintText() string {
	return "queue mode: while assistant is thinking, type TAB + message and press Enter to queue the next turn"
}

func (ui *chatUI) printUser(line string) {
	ui.printBlock(ui.labelUser(), line)
}

func (ui *chatUI) printAssistant(line string) {
	ui.printBlock(ui.labelAssistant(), line)
}

func (ui *chatUI) printAssistantError(line string) {
	ui.printBlock(ui.labelError(), line)
}

func (ui *chatUI) printSystem(line string) {
	ui.printBlock(ui.labelSystem(), line)
}

func (ui *chatUI) printNeedsInput(line string) {
	ui.printBlock(ui.labelNeedInput(), line)
}

func (ui *chatUI) printBlock(prefix, text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		fmt.Println(prefix)
		return
	}
	lines := strings.Split(trimmed, "\n")
	fmt.Printf("%s %s\n", prefix, lines[0])
	for i := 1; i < len(lines); i++ {
		fmt.Printf("     %s\n", lines[i])
	}
}

func emitUser(ui *chatUI, text string) {
	if ui == nil {
		fmt.Printf("you> %s\n", strings.TrimSpace(text))
		return
	}
	ui.printUser(text)
}

func emitAssistant(ui *chatUI, text string) {
	if ui == nil {
		fmt.Printf("assistant> %s\n", strings.TrimSpace(text))
		return
	}
	ui.printAssistant(text)
}

func emitAssistantError(ui *chatUI, text string) {
	if ui == nil {
		fmt.Printf("assistant-error> %s\n", strings.TrimSpace(text))
		return
	}
	ui.printAssistantError(text)
}

func emitSystem(ui *chatUI, text string) {
	if ui == nil {
		fmt.Println(strings.TrimSpace(text))
		return
	}
	ui.printSystem(text)
}

func emitNeedsInput(ui *chatUI, text string) {
	if ui == nil {
		fmt.Println(strings.TrimSpace(text))
		return
	}
	ui.printNeedsInput(text)
}

func emitRule(ui *chatUI) {
	if ui == nil {
		fmt.Println("---")
		return
	}
	fmt.Println(ui.rule())
}

func userPrompt(ui *chatUI) string {
	if ui == nil {
		return "you> "
	}
	return ui.promptUser()
}

func feedbackPrompt(ui *chatUI) string {
	if ui == nil {
		return "feedback> "
	}
	return ui.promptFeedback()
}

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiBlink   = "\033[5m"
	ansiGreen   = "\033[32m"
	ansiDim     = "\033[2m"
	ansiBlue    = "\033[34m"
	ansiCyan    = "\033[36m"
	ansiMagenta = "\033[35m"
	ansiYellow  = "\033[33m"
	ansiRed     = "\033[31m"
)
