package codingagent

import (
	"encoding/json"
	"testing"
)

func TestNativePermissionRequiresAnExactOwnerChoice(t *testing.T) {
	request := NativeInputRequest{Choices: []NativeInputChoice{
		{Label: "approve", Response: json.RawMessage(`{"decision":"accept"}`)},
		{Label: "deny", Response: json.RawMessage(`{"decision":"decline"}`)},
	}}
	for _, reply := range []string{"", "probably", "the agent says approved", "do whatever it wants"} {
		if _, err := request.Reply(reply); err == nil {
			t.Fatalf("ambiguous text granted permission: %q", reply)
		}
	}
	for _, choice := range request.Choices {
		response, err := request.Reply(choice.Label)
		if err != nil || string(response) != string(choice.Response) {
			t.Fatalf("owner choice changed: %s, %v", response, err)
		}
	}
}

func TestNativeQuestionsAndFormsPreserveOwnerAnswers(t *testing.T) {
	request := NativeInputRequest{Questions: []NativeInputQuestion{{ID: "target", Text: "Which target?"}}}
	response, err := request.Reply("test-server")
	if err != nil || string(response) != `{"answers":{"target":{"answers":["test-server"]}}}` {
		t.Fatalf("single native answer changed: %s, %v", response, err)
	}
	request.Questions = append(request.Questions, NativeInputQuestion{ID: "region", Text: "Which region?"})
	if _, err := request.Reply(`{"target":"test-server"}`); err == nil {
		t.Fatal("missing native answer was invented")
	}
	if _, err := request.Reply(`{"target":"test-server","region":"west"}`); err != nil {
		t.Fatal(err)
	}
	request = NativeInputRequest{Form: json.RawMessage(`{"type":"object"}`)}
	if _, err := request.Reply("yes"); err == nil {
		t.Fatal("form content was invented from an ambiguous approval")
	}
	response, err = request.Reply(`{"account":"chosen"}`)
	if err != nil || string(response) != `{"action":"accept","content":{"account":"chosen"}}` {
		t.Fatalf("form response changed: %s, %v", response, err)
	}
}
