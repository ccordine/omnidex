package queue

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestScrumChannelEffectExposesNoCallerSelectedTransportOrMetadata(t *testing.T) {
	typeOfEffect := reflect.TypeOf(ScrumChannelEffect{})
	for _, forbidden := range []string{"Pipeline", "Metadata", "Agent", "AgentConfig", "Path", "Tool"} {
		if _, exists := typeOfEffect.FieldByName(forbidden); exists {
			t.Fatalf("Scrum channel effect exposes caller-selected %s", forbidden)
		}
	}
}

func TestScrumChannelOperationRequiresExplicitTypedIdentity(t *testing.T) {
	_, err := describeScrumChannelOperation(ScrumChannelOperationRequest{
		ProjectID: 4, CardID: "card-4", Message: "Continue.",
	})
	if err == nil {
		t.Fatal("missing lifecycle operation identity must fail")
	}
}

func TestScrumChannelOperationPreservesExactNonblankMessage(t *testing.T) {
	operationID, err := NewLifecycleOperationID("scrum-channel-exact-message")
	if err != nil {
		t.Fatal(err)
	}
	exact := "  Preserve exact channel bytes.\n"
	descriptor, err := describeScrumChannelOperation(ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: exact,
	})
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Request.Message != exact {
		t.Fatalf("message=%q want exact=%q", descriptor.Request.Message, exact)
	}
}

func TestScrumChannelOperationUsesExactUserTurnByteBoundary(t *testing.T) {
	if model.MaxFreeFormTurnBytes != 4096 {
		t.Fatalf("model user-turn bound=%d want=4096", model.MaxFreeFormTurnBytes)
	}
	operationID, err := NewLifecycleOperationID("scrum-channel-user-turn-bound")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		message string
		wantErr bool
	}{
		{name: "exactly 4096 bytes", message: strings.Repeat("x", model.MaxFreeFormTurnBytes)},
		{name: "4097 bytes", message: strings.Repeat("x", model.MaxFreeFormTurnBytes+1), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := describeScrumChannelOperation(ScrumChannelOperationRequest{
				OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: test.message,
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("message bytes=%d error=%v wantErr=%t", len(test.message), err, test.wantErr)
			}
		})
	}
}

func TestScrumChannelOperationSharesModelUnicodeBlankSemantics(t *testing.T) {
	operationID, err := NewLifecycleOperationID("scrum-channel-unicode-blank")
	if err != nil {
		t.Fatal(err)
	}
	unicodeWhiteSpace := []rune{
		'\u0009', '\u000a', '\u000b', '\u000c', '\u000d', '\u0020', '\u0085', '\u00a0',
		'\u1680', '\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006',
		'\u2007', '\u2008', '\u2009', '\u200a', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000',
	}
	for _, whitespace := range unicodeWhiteSpace {
		t.Run(fmt.Sprintf("U+%04X", whitespace), func(t *testing.T) {
			message := string(whitespace)
			modelErr := model.ValidateChannelMessage(model.ChannelMessageRoleUser, message)
			_, scrumErr := describeScrumChannelOperation(ScrumChannelOperationRequest{
				OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: message,
			})
			if modelErr == nil {
				t.Fatalf("model accepted Unicode White_Space rune U+%04X", whitespace)
			}
			if scrumErr == nil {
				t.Fatalf("Scrum accepted Unicode White_Space rune U+%04X", whitespace)
			}
		})
	}

	const byteOrderMark = "\ufeff"
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, byteOrderMark); err != nil {
		t.Fatalf("model rejected U+FEFF, which is not in the Unicode White_Space property: %v", err)
	}
	if _, err := describeScrumChannelOperation(ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: byteOrderMark,
	}); err != nil {
		t.Fatalf("Scrum diverged from model semantics for U+FEFF: %v", err)
	}
}

func TestScrumChannelOperationDigestPreservesHostileJSONText(t *testing.T) {
	operationID, err := NewLifecycleOperationID("scrum-channel-hostile-json")
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		message string
		payload string
		digest  string
	}{
		{
			message: "<script>&\u2028\u2029\"\\\n",
			payload: `{"operation_id":"lifecycle_operation_d3309a5f031fa87bb7d0d0faf345baf23fa92291b4f800553b6ad3340c476a3e","project_id":4,"card_id":"card-4","message":"\u003cscript\u003e\u0026\u2028\u2029\"\\\n"}`,
			digest:  "9b46035e083f7fa4390867f447d752f8f14f81c974e1940bb75a261463514f37",
		},
		{
			message: `\u003cscript\u003e\u0026\u2028`,
			payload: `{"operation_id":"lifecycle_operation_d3309a5f031fa87bb7d0d0faf345baf23fa92291b4f800553b6ad3340c476a3e","project_id":4,"card_id":"card-4","message":"\\u003cscript\\u003e\\u0026\\u2028"}`,
			digest:  "5675ad91b36a3c3ac7294cc069d270655d4c218dc21acc024759829f69ae0e1d",
		},
	}
	digests := make(map[string]struct{}, len(fixtures))
	for _, fixture := range fixtures {
		request := ScrumChannelOperationRequest{
			OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: fixture.message,
		}
		descriptor, describeErr := describeScrumChannelOperation(request)
		if describeErr != nil {
			t.Fatal(describeErr)
		}
		if string(descriptor.Payload) != fixture.payload || descriptor.Request.Message != fixture.message {
			t.Fatalf("descriptor rewrote hostile message: payload=%s expected=%s", descriptor.Payload, fixture.payload)
		}
		var decoded ScrumChannelOperationRequest
		if err := json.Unmarshal(descriptor.Payload, &decoded); err != nil || decoded.Message != fixture.message {
			t.Fatalf("decode hostile payload=%+v error=%v", decoded, err)
		}
		if descriptor.SHA256 != fixture.digest {
			t.Fatalf("digest=%q want=%q", descriptor.SHA256, fixture.digest)
		}
		digests[descriptor.SHA256] = struct{}{}
	}
	if len(digests) != len(fixtures) {
		t.Fatal("byte-distinct hostile messages produced the same command digest")
	}
}

func TestScrumChannelOperationRejectsUnregisteredOrMixedEffects(t *testing.T) {
	operationID, err := NewLifecycleOperationID("scrum-channel-types")
	if err != nil {
		t.Fatal(err)
	}
	base := ScrumChannelOperationCommand{
		Request: ScrumChannelOperationRequest{
			OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: "Continue.",
		},
		ExpectedCardUpdatedAt: time.Now(), ResultAction: "started",
		Effect: ScrumChannelEffect{
			Kind: ScrumChannelStartJob, Instruction: "Continue.",
		},
	}
	if _, _, err := normalizeScrumChannelOperation(base); err != nil {
		t.Fatalf("valid start operation: %v", err)
	}
	mixed := base
	mixed.Effect.Kind = ScrumChannelReplanJob
	mixed.Effect.JobID = 19
	if _, _, err := normalizeScrumChannelOperation(mixed); err == nil {
		t.Fatal("replan effect with start-job fields must fail")
	}
	unregistered := base
	unregistered.Effect.Kind = "unknown"
	if _, _, err := normalizeScrumChannelOperation(unregistered); err == nil {
		t.Fatal("unregistered effect must fail")
	}
	mismatched := base
	mismatched.Effect.Instruction = "Different hidden instruction."
	if _, _, err := normalizeScrumChannelOperation(mismatched); err == nil {
		t.Fatal("start effect with instruction outside the exact request must fail")
	}
}

func TestScrumChannelOperationRejectsNoncanonicalAuthorityText(t *testing.T) {
	operationID, err := NewLifecycleOperationID("scrum-channel-noncanonical")
	if err != nil {
		t.Fatal(err)
	}
	base := ScrumChannelOperationCommand{
		Request: ScrumChannelOperationRequest{
			OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: "Continue.",
		},
		ExpectedCardUpdatedAt: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
		ResultAction:          "started",
		Effect: ScrumChannelEffect{
			Kind: ScrumChannelStartJob, Instruction: "Continue.",
		},
	}
	for name, mutate := range map[string]func(*ScrumChannelOperationCommand){
		"card ID":       func(command *ScrumChannelOperationCommand) { command.Request.CardID = " card-4" },
		"result action": func(command *ScrumChannelOperationCommand) { command.ResultAction = "started " },
	} {
		t.Run(name, func(t *testing.T) {
			command := base
			mutate(&command)
			if _, _, err := normalizeScrumChannelOperation(command); err == nil {
				t.Fatal("noncanonical authority text was accepted")
			}
		})
	}
	update := scrumChannelTestUpdate(t, DBScrumCard{}, base.Request, model.Job{ID: 9})
	update.Column = "in_progress "
	if err := validateScrumChannelCardUpdate(base.Request, model.Job{ID: 9}, &update); err == nil {
		t.Fatal("noncanonical card update authority was accepted")
	}
}
