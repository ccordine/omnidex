package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"golang.org/x/term"
)

func TestChatConsoleComposesTypedAndPastedText(t *testing.T) {
	for _, test := range []struct {
		name, input, want string
	}{
		{"prefix and suffix", "Before " + terminalPaste("first\nsecond") + " after", "Before first\nsecond after"},
		{"edit pasted text", terminalPaste("first\nsecondx") + "\x7f!", "first\nsecond!"},
		{"paste at cursor", "AB\x1b[D" + terminalPaste("one\r\ntwo\t"), "Aone\r\ntwo\tB"},
		{"edit newline", terminalPaste("a\nb") + "\x1b[D\x7f ", "a b"},
		{"multiple pastes", "前 " + terminalPaste("one\n") + "中 " + terminalPaste("two\rthree\t") + "後", "前 one\n中 two\rthree\t後"},
		{"literal display glyphs", "↵" + terminalPaste("␍\n⇥"), "↵␍\n⇥"},
		{"pasted slash stays text", "/" + terminalPaste("exit"), "/exit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			console, _ := pasteTestConsole(test.input + "\r/exit\r")
			line, pasted, _, err := console.ReadLine()
			if err != nil || line != test.want || !pasted {
				t.Fatalf("composed input = %q, pasted %t, error %v; want %q", line, pasted, err, test.want)
			}
			line, pasted, _, err = console.ReadLine()
			if err != nil || line != "/exit" || pasted {
				t.Fatalf("next input = %q, pasted %t, error %v", line, pasted, err)
			}
		})
	}
}

func TestChatConsolePasteBoundsAndInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name, input string
	}{
		{"invalid UTF8", "prefix" + terminalPaste("\xff")},
		{"escape control", "prefix" + terminalPaste("\x1b[31m")},
		{"pasted LF marker", "prefix" + terminalPaste("\u2028")},
		{"pasted CR marker", "prefix" + terminalPaste("\u2029")},
		{"pasted tab marker", "prefix" + terminalPaste("\u0085")},
		{"typed LF marker", "prefix\u2028" + terminalPaste("tail")},
		{"typed CR marker", "prefix\u2029" + terminalPaste("tail")},
		{"typed tab marker", "prefix\u0085" + terminalPaste("tail")},
	} {
		t.Run(test.name, func(t *testing.T) {
			console, _ := pasteTestConsole(test.input + "\rnext\r")
			line, _, _, err := console.ReadLine()
			if !errors.Is(err, errChatInputRejected) || line != "" {
				t.Fatalf("unsafe input = %q, error %v", line, err)
			}
			line, pasted, _, err := console.ReadLine()
			if err != nil || line != "next" || pasted {
				t.Fatalf("input after rejection = %q, pasted %t, error %v", line, pasted, err)
			}
		})
	}
}

func TestChatConsoleMixedPastePreservesLargeInput(t *testing.T) {
	want := "x" + strings.Repeat("\n", model.MaxFreeFormTurnBytes+1)
	console, _ := pasteTestConsole("x" + terminalPaste(want[1:]) + "\r")
	line, pasted, _, err := console.ReadLine()
	if err != nil || line != want || !pasted {
		t.Fatalf("boundary input = %d bytes, pasted %t, error %v", len(line), pasted, err)
	}
}

func TestChatConsolePreservesLongTypedAndPastedDrafts(t *testing.T) {
	for _, text := range []string{
		strings.Repeat("界", 4100) + " tail",
		"Create sample.txt containing the following supplied text exactly. Do not add other files.\n\n" + strings.Repeat("Example line for the cancellation and input-size pilot.\n", 600),
	} {
		for _, pasted := range []bool{false, true} {
			if !pasted && strings.Contains(text, "\n") {
				continue
			}
			input := text
			if pasted {
				input = terminalPaste(text)
			}
			console, _ := pasteTestConsole(input + "\r")
			got, _, _, err := console.ReadLine()
			if err != nil || got != text {
				t.Fatalf("pasted=%t: received %d/%d bytes: %v", pasted, len(got), len(text), err)
			}
		}
	}
}

func TestChatConsolePasteProvenanceAcrossLineEndings(t *testing.T) {
	for _, ending := range []string{"\r", "\n", "\r\n"} {
		t.Run(ending, func(t *testing.T) {
			console, _ := pasteTestConsole("first" + ending + terminalPaste("/exit") + ending)
			line, pasted, _, err := console.ReadLine()
			if err != nil || line != "first" || pasted {
				t.Fatalf("typed turn = %q, pasted %t, error %v", line, pasted, err)
			}
			line, pasted, _, err = console.ReadLine()
			if err != nil || line != "/exit" || !pasted {
				t.Fatalf("pasted turn = %q, pasted %t, error %v", line, pasted, err)
			}
		})
	}
}

func TestChatConsolePasteProjectionIsSafeAndEditableFromHistory(t *testing.T) {
	console, output := pasteTestConsole(terminalPaste("first\r\nsecond\t") + "\r\x1b[A!\r")
	for _, want := range []string{"first\r\nsecond\t", "first\r\nsecond\t!"} {
		line, pasted, _, err := console.ReadLine()
		if err != nil || line != want || !pasted {
			t.Fatalf("history turn = %q, pasted %t, error %v", line, pasted, err)
		}
	}
	if strings.ContainsAny(output.String(), "\u2028\u2029\u0085") {
		t.Fatal("internal paste controls reached terminal output")
	}
}

func TestChatConsoleMixedPasteSubmitsExactTextInsteadOfCommand(t *testing.T) {
	for _, input := range []string{
		"before " + terminalPaste("one\r\ntwo\t") + " after",
		"/" + terminalPaste("exit"),
	} {
		t.Run(input, func(t *testing.T) {
			console, _ := pasteTestConsole(input + "\r/exit\r")
			line, pasted, _, err := console.ReadLine()
			if err != nil {
				t.Fatal(err)
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var payload struct {
					Text string `json:"text"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Text != line {
					t.Errorf("submitted text = %q, error %v; want %q", payload.Text, err, line)
				}
				http.Error(w, "fixture received exact turn", http.StatusBadRequest)
			}))
			defer server.Close()
			session := newChatOperationTestSession(t, server.URL)
			quit, err := session.acceptInput(line, pasted)
			if quit || !client.IsHTTPStatus(err, http.StatusBadRequest) || requests != 1 {
				t.Fatalf("paste submission = quit %t, requests %d, error %v", quit, requests, err)
			}
			line, pasted, _, err = console.ReadLine()
			if err != nil {
				t.Fatal(err)
			}
			quit, err = session.acceptInput(line, pasted)
			if !quit || err != nil || requests != 1 {
				t.Fatalf("typed exit = quit %t, requests %d, error %v", quit, requests, err)
			}
		})
	}
}

func terminalPaste(value string) string {
	return "\x1b[200~" + value + "\x1b[201~"
}

func pasteTestConsole(input string) (*chatConsole, *bytes.Buffer) {
	reader := newBracketedPasteReader(strings.NewReader(input))
	var output bytes.Buffer
	console := &chatConsole{pasteReader: reader, output: &output, terminalFD: -1}
	console.terminal = term.NewTerminal(terminalReadWriter{reader: reader, writer: &output}, "you> ")
	console.terminal.SetBracketedPasteMode(true)
	return console, &output
}
