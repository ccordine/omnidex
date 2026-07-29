package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

func runChat(c *client.Client, args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	sessionID := fs.String("session", "", "session/thread identifier for this interactive chat")
	interval := fs.Duration("interval", 2*time.Second, "poll interval while waiting for each turn")
	progress := fs.Bool("progress", true, "print live stage/event updates while waiting for each turn")
	verbose := fs.Bool("verbose", false, "print full debug trace (including LLM prompts and full context dumps) while waiting")
	maxChars := fs.Int("max-chars", 1200, "max characters shown per streamed LLM/context entry (0 disables truncation)")
	modelAnalyze := fs.String("model-analyze", "", "override analyze model for this chat session")
	modelResponse := fs.String("model-response", "", "override response model for this chat session")
	modelSearch := fs.String("model-search", "", "override search-query model for this chat session")
	modelTagger := fs.String("model-tagger", "", "override tagging model for this chat session")
	modelPlan := fs.String("model-plan", "", "override planner model for this chat session")
	modelVerify := fs.String("model-verify", "", "override verification evaluator model for this chat session")
	modelMemory := fs.String("model-memory", "", "override memory-inference model for this chat session")
	agentFlagPointers := registerCLIAgentRuntimeFlags(fs)
	_ = fs.Parse(args)
	agentOverrides, err := cliAgentRuntimeConfigFromFlags(agentFlagPointers.Values())
	if err != nil {
		die(err.Error())
	}

	baseMetadata := map[string]any{}
	chatCWD := ""
	if dir, err := os.Getwd(); err == nil && strings.TrimSpace(dir) != "" {
		chatCWD = strings.TrimSpace(dir)
	}
	session := strings.TrimSpace(*sessionID)
	if session == "" {
		session = defaultProjectScopedSessionID(chatCWD)
	}
	hostSnapshot := discoverHostEnvironmentSnapshot(chatCWD)
	applyHostEnvironmentMetadata(baseMetadata, hostSnapshot)
	applyHostTemporalMetadata(baseMetadata, time.Now())
	if strings.TrimSpace(*modelAnalyze) != "" {
		baseMetadata["model_analyze"] = strings.TrimSpace(*modelAnalyze)
	}
	if strings.TrimSpace(*modelResponse) != "" {
		baseMetadata["model_response"] = strings.TrimSpace(*modelResponse)
	}
	if strings.TrimSpace(*modelSearch) != "" {
		baseMetadata["model_search"] = strings.TrimSpace(*modelSearch)
	}
	if strings.TrimSpace(*modelTagger) != "" {
		baseMetadata["model_tagger"] = strings.TrimSpace(*modelTagger)
	}
	if strings.TrimSpace(*modelPlan) != "" {
		baseMetadata["model_plan"] = strings.TrimSpace(*modelPlan)
	}
	if strings.TrimSpace(*modelVerify) != "" {
		baseMetadata["model_verify"] = strings.TrimSpace(*modelVerify)
	}
	if strings.TrimSpace(*modelMemory) != "" {
		baseMetadata["model_memory"] = strings.TrimSpace(*modelMemory)
	}
	agentOverrides.ApplyToMetadata(baseMetadata)
	if err := persistHostCapabilityMemory(c, hostSnapshot); err != nil {
		fmt.Fprintf(os.Stderr, "warn: capability memory sync failed: %v\n", err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	input := newChatInputReader(scanner)

	lastJobID := int64(0)
	pendingInputs := make([]string, 0, 4)
	if initialInput := strings.TrimSpace(strings.Join(fs.Args(), " ")); initialInput != "" {
		pendingInputs = append(pendingInputs, initialInput)
	}
	ui := newChatUI()
	ui.printBanner(session)
	if len(agentOverrides.ToMap()) > 0 {
		emitSystem(ui, agentOverrides.Summary())
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
			line = strings.TrimSpace(rawLine)
		}

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			handled, quit := handleChatReplCommand(line, &session, &lastJobID, baseMetadata, agentOverrides, ui)
			if quit {
				return
			}
			if handled {
				continue
			}
		}

		quit := executeChatCoreTurn(
			c,
			input,
			session,
			baseMetadata,
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
