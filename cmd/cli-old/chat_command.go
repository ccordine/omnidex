package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func runChat(c *client.Client, args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	sessionID := fs.String("session", "", "session/thread identifier for this interactive chat")
	interval := fs.Duration("interval", 2*time.Second, "poll interval while waiting for each turn")
	progress := fs.Bool("progress", true, "print live stage/event updates while waiting for each turn")
	verbose := fs.Bool("verbose", false, "print full debug trace (including LLM prompts and full context dumps) while waiting")
	maxChars := fs.Int("max-chars", 1200, "max characters shown per streamed LLM/context entry (0 disables truncation)")
	_ = fs.Parse(args)

	chatCWD, err := os.Getwd()
	if err != nil {
		die("resolve chat workspace: " + err.Error())
	}
	if err := model.ValidateChannelWorkspaceRoot(chatCWD); err != nil {
		die("resolve chat workspace: " + err.Error())
	}
	session := strings.TrimSpace(*sessionID)
	if session == "" {
		session = defaultProjectScopedSessionID(chatCWD)
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	input := newChatInputReader(scanner)

	lastJobID := int64(0)
	pendingInputs := make([]string, 0, 4)
	if initialInput, ok := initialChatInstruction(fs.Args()); ok {
		pendingInputs = append(pendingInputs, initialInput)
	}
	ui := newChatUI()
	ui.printBanner(session)
	channel, err := ensureChatChannel(context.Background(), c, session, chatCWD)
	if err != nil {
		die(err.Error())
	}

	for {
		var line string
		if len(pendingInputs) > 0 {
			line = pendingInputs[0]
			pendingInputs = pendingInputs[1:]
			emitUser(ui, line)
		} else {
			fmt.Print(userPrompt(ui))
			rawLine, eof, err := input.readBlocking()
			if err != nil {
				die(err.Error())
			}
			if eof {
				fmt.Println("")
				return
			}
			line = rawLine
		}

		if !isNonBlankChatInstruction(line) {
			continue
		}

		commandLine := strings.TrimSpace(line)
		if strings.HasPrefix(commandLine, "/") {
			previousSession := session
			handled, quit := handleChatReplCommand(commandLine, &session, &lastJobID, ui)
			if quit {
				return
			}
			if handled {
				if session != previousSession {
					channel, err = ensureChatChannel(context.Background(), c, session, chatCWD)
					if err != nil {
						die(err.Error())
					}
				}
				continue
			}
		}

		quit := executeChatCoreTurn(
			c,
			input,
			channel.ID,
			&lastJobID,
			&pendingInputs,
			line,
			*interval,
			*progress,
			*verbose,
			*maxChars,
			ui,
		)
		if quit {
			return
		}
	}
}

func initialChatInstruction(arguments []string) (string, bool) {
	instruction := strings.Join(arguments, " ")
	return instruction, isNonBlankChatInstruction(instruction)
}

func isNonBlankChatInstruction(instruction string) bool {
	return strings.TrimSpace(instruction) != ""
}
