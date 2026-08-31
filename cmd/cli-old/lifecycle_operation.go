package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func newLifecycleOperationID() queue.LifecycleOperationID {
	id, err := queue.NewRandomLifecycleOperationID()
	if err != nil {
		die(err.Error())
	}
	announceLifecycleOperationID(id)
	return id
}

func parseLifecycleOperationArgs(args []string) (queue.LifecycleOperationID, []string, error) {
	remaining := make([]string, 0, len(args))
	var operationID queue.LifecycleOperationID
	for index := 0; index < len(args); index++ {
		argument := args[index]
		inlineValue := ""
		isInline := strings.HasPrefix(argument, "--operation-id=")
		if isInline {
			inlineValue = strings.TrimPrefix(argument, "--operation-id=")
		}
		if argument != "--operation-id" && !isInline {
			remaining = append(remaining, args[index])
			continue
		}
		if operationID != "" {
			return "", nil, fmt.Errorf("--operation-id may be supplied exactly once")
		}
		value := inlineValue
		if !isInline {
			if index+1 >= len(args) {
				return "", nil, fmt.Errorf("--operation-id requires a lifecycle operation ID")
			}
			value = args[index+1]
			index++
		}
		parsed, err := queue.ParseLifecycleOperationID(value)
		if err != nil {
			return "", nil, err
		}
		operationID = parsed
	}
	if operationID == "" {
		var err error
		operationID, err = queue.NewRandomLifecycleOperationID()
		if err != nil {
			return "", nil, fmt.Errorf("generate lifecycle operation ID: %w", err)
		}
	}
	return operationID, remaining, nil
}

func announceLifecycleOperationID(operationID queue.LifecycleOperationID) {
	fmt.Fprintf(os.Stderr, "lifecycle operation id: %s\n", operationID)
}
