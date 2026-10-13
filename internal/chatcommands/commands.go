// Package chatcommands owns the command meanings shared by CLI help and
// conversation responses about exact CLI command references.
package chatcommands

import (
	"fmt"
	"strings"
)

type Name string

const (
	Status    Name = "status"
	Verbosity Name = "verbosity"
	Interrupt Name = "interrupt"
	Redirect  Name = "redirect"
	Cancel    Name = "cancel"
	Stop      Name = "stop"
	Exit      Name = "exit"
)

type Definition struct {
	Name      Name
	Arguments string
	Meaning   string
}

var definitions = [...]Definition{
	{Status, "", "show current work"},
	{Verbosity, " <level>", "quiet, normal, verbose, or debug"},
	{Interrupt, " [reason]", "pause the active job on the same job identity"},
	{Redirect, " <text>", "explicitly redirect the active job"},
	{Cancel, " [reason]", "stop the active task"},
	{Stop, "", "stop the active task"},
	{Exit, "", "close this client; server work continues"},
}

func Count() int { return len(definitions) }

func Lookup(name Name) (Definition, bool) {
	for _, definition := range definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}

// LookupReference recognizes only the complete registered spelling. A CLI
// meaning is not a statement about whether the same spelling names a file.
func LookupReference(reference string) (Definition, bool) {
	name, present := strings.CutPrefix(reference, "/")
	if !present {
		return Definition{}, false
	}
	return Lookup(Name(name))
}

func Help() string {
	var help strings.Builder
	help.WriteString("commands:\n")
	for _, definition := range definitions {
		fmt.Fprintf(&help, "  %-24s%s\n", "/"+string(definition.Name)+definition.Arguments, definition.Meaning)
	}
	help.WriteString("Reply while working to give feedback; /redirect changes the plan.\n")
	return help.String()
}
