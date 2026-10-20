package chatcommands

import "testing"

func TestCanonicalChatHelpPreservesExistingOutput(t *testing.T) {
	want := "commands:\n" +
		"  /mode <name>            Infer, Interact, Ledger, or Research\n" +
		"  /status                 show current work\n" +
		"  /verbosity <level>      quiet, normal, verbose, or debug\n" +
		"  /interrupt [reason]     pause the active job on the same job identity\n" +
		"  /redirect <text>        explicitly redirect the active job\n" +
		"  /cancel [reason]        stop the active task\n" +
		"  /stop                   stop the active task\n" +
		"  /exit                   close this client; server work continues\n" +
		"Reply while working to give feedback; /redirect changes the plan.\n"
	if got := Help(); got != want {
		t.Fatalf("canonical registry changed CLI help:\ngot %q\nwant %q", got, want)
	}
}

func TestCommandReferenceLookupRequiresCompleteRegisteredSpelling(t *testing.T) {
	for _, reference := range []string{"/interrupt/notes", "/STOP", "stop", "/stop extra", "/stop?", "//stop"} {
		if _, recognized := LookupReference(reference); recognized {
			t.Errorf("recognized unregistered reference %q", reference)
		}
	}
	for reference, name := range map[string]Name{"/interrupt": Interrupt, "/stop": Stop, "/cancel": Cancel} {
		definition, recognized := LookupReference(reference)
		if !recognized || definition.Name != name || definition.Meaning == "" {
			t.Errorf("registered reference %q lacks its canonical meaning: %+v", reference, definition)
		}
	}
}
