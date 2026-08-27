package worker

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	contextRecentRecordLimit   = 6
	contextSearchedRecordLimit = 8
)

type boundObjectiveContextProvider struct {
	runtime     *nativeRuntimeV3
	job         model.Job
	authority   turnAuthority
	preparation *roleplay.SimulationTurnAuthority
	projection  *roleplay.NarrativeSimulationProjection
}

func (provider boundObjectiveContextProvider) validateAuthority() error {
	if provider.runtime == nil || provider.runtime.svc == nil || provider.runtime.svc.repo == nil {
		return fmt.Errorf("context retrieval requires repository authority")
	}
	if provider.job.ID != provider.authority.JobID ||
		provider.job.Instruction != provider.authority.Instruction {
		return fmt.Errorf("context retrieval differs from exact turn authority")
	}
	return nil
}

func (provider boundObjectiveContextProvider) Retrieve(
	ctx context.Context,
	terms []string,
) (contextcompiler.CandidateSet, error) {
	if err := provider.validateAuthority(); err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	continuity, err := provider.runtime.svc.repo.ObjectiveContinuityAuthorities(ctx, provider.job)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	if provider.authority.ChannelMode == model.ChannelModeRoleplay {
		return provider.retrieveRoleplay(ctx, terms, continuity.Replan)
	}
	return provider.retrieveAssistant(ctx, terms, continuity)
}

func (provider boundObjectiveContextProvider) retrieveAssistant(
	ctx context.Context,
	terms []string,
	continuity queue.ObjectiveContinuityAuthority,
) (contextcompiler.CandidateSet, error) {
	recent, err := provider.runtime.svc.repo.ConversationCandidateAuthorities(ctx, provider.job)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	recentRecords, err := recentConversationContextRecords(recent.Turns)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	searched := []queue.ContextSearchRecord{}
	memory := []queue.ContextSearchRecord{}
	if len(terms) != 0 {
		searched, err = provider.runtime.svc.repo.SearchConversationContextRecords(
			ctx, provider.job, terms, contextSearchedRecordLimit,
		)
		if err != nil {
			return contextcompiler.CandidateSet{}, err
		}
		for index := range searched {
			if searched[index].Namespace != "conversation_exchange" {
				return contextcompiler.CandidateSet{}, fmt.Errorf(
					"assistant context search returned unregistered namespace %q",
					searched[index].Namespace,
				)
			}
		}
		memory, err = provider.retrieveDurableMemory(ctx, continuity.Scope, terms)
		if err != nil {
			return contextcompiler.CandidateSet{}, fmt.Errorf(
				"retrieve assistant durable memory context: %w", err,
			)
		}
	}
	// Keep every fixed provider represented before later candidates from any
	// single provider can consume the hard relevance projection budget.
	records := interleaveContextRecordGroups(recentRecords, searched, memory)
	set, err := buildContextCandidateSet(
		replanContextRecords(continuity.Replan), records,
	)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	set.Replan = continuity.Replan
	return set, nil
}

func (provider boundObjectiveContextProvider) retrieveDurableMemory(
	ctx context.Context,
	scope *model.MemoryScope,
	terms []string,
) ([]queue.ContextSearchRecord, error) {
	if scope == nil || len(terms) == 0 {
		return []queue.ContextSearchRecord{}, nil
	}
	hasMemory, err := provider.runtime.svc.repo.HasScopedMemory(ctx, *scope)
	if err != nil || !hasMemory {
		return nil, err
	}
	if provider.runtime.svc.embeddings == nil {
		return nil, fmt.Errorf("scoped context retrieval requires embedding authority")
	}
	embeddings, err := embedContextQueries(ctx, provider.runtime.svc.embeddings, terms)
	if err != nil {
		return nil, err
	}
	groups := make([][]model.MemoryMatch, 0, len(embeddings))
	for _, embedding := range embeddings {
		matches, err := provider.runtime.svc.repo.FindRelevantMemory(
			ctx, *scope, embedding, assemblyline.MaxMemoryContextCandidateAuthorities,
		)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			if match.Scope != *scope {
				return nil, fmt.Errorf("memory context retrieval escaped its exact scope")
			}
		}
		groups = append(groups, matches)
	}
	matches, err := roundRobinMemoryMatches(groups, contextSearchedRecordLimit)
	if err != nil {
		return nil, err
	}
	records := make([]queue.ContextSearchRecord, len(matches))
	for index, match := range matches {
		records[index] = queue.ContextSearchRecord{
			Namespace: "durable_memory", SourceID: fmt.Sprintf("memory-%d", match.ID),
			Content: match.Content,
		}
	}
	return records, nil
}

// roundRobinMemoryMatches gives every canonical query one opportunity to
// contribute a unique exact memory before any query contributes another.
// Duplicate rows are skipped within that query's turn, so overlap cannot let
// an earlier query consume the entire global bound.
func roundRobinMemoryMatches(
	groups [][]model.MemoryMatch,
	limit int,
) ([]model.MemoryMatch, error) {
	if limit < 1 || limit > assemblyline.MaxMemoryContextCandidateAuthorities {
		return nil, fmt.Errorf(
			"memory context merge limit must be within 1..%d",
			assemblyline.MaxMemoryContextCandidateAuthorities,
		)
	}
	positions := make([]int, len(groups))
	seen := make(map[int64]model.MemoryMatch, limit)
	merged := make([]model.MemoryMatch, 0, limit)
	for len(merged) < limit {
		progress := false
		for groupIndex, group := range groups {
			for positions[groupIndex] < len(group) {
				match := group[positions[groupIndex]]
				positions[groupIndex]++
				if match.ID < 1 {
					return nil, fmt.Errorf("memory context retrieval returned an invalid identity")
				}
				if previous, duplicate := seen[match.ID]; duplicate {
					if !sameMemoryMatchAuthority(previous, match) {
						return nil, fmt.Errorf(
							"memory context retrieval returned conflicting authority for memory %d",
							match.ID,
						)
					}
					continue
				}
				seen[match.ID] = match
				merged = append(merged, match)
				progress = true
				break
			}
			if len(merged) == limit {
				break
			}
		}
		if !progress {
			break
		}
	}
	return merged, nil
}

func sameMemoryMatchAuthority(left, right model.MemoryMatch) bool {
	return left.ID == right.ID && left.Scope == right.Scope && left.Kind == right.Kind &&
		left.Content == right.Content && left.CreatedAt.Equal(right.CreatedAt) &&
		slices.Equal(left.Tags, right.Tags) && slices.Equal(left.Categories, right.Categories)
}

func (provider boundObjectiveContextProvider) retrieveRoleplay(
	ctx context.Context,
	terms []string,
	replan *assemblyline.ObjectiveReplanAuthority,
) (contextcompiler.CandidateSet, error) {
	preparation, _, responder, err := provider.roleplayContextAuthority()
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	conversation, err := provider.runtime.svc.repo.RoleplayConversationCandidateAuthorities(
		ctx, provider.job, model.RoleplayCharacterID(responder.CharacterID),
	)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	completedTurns, err := completedConversationCandidateTurns(conversation.Turns)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	conversationRecords, err := recentConversationContextRecords(completedTurns)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	requiredRecords, optionalConversation, rankableFrozen, err := roleplayContextRecordGroups(
		preparation, responder, conversationRecords, replan,
	)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	historical := []queue.ContextSearchRecord{}
	if len(terms) != 0 {
		historical, err = provider.runtime.svc.repo.SearchRoleplayContextRecords(
			ctx, preparation.WorldID, model.RoleplayCharacterID(responder.CharacterID),
			preparation.SceneID, preparation.CreatedAt, terms, contextSearchedRecordLimit,
		)
		if err != nil {
			return contextcompiler.CandidateSet{}, err
		}
	}
	currentRound, err := currentRoundResponseContextRecords(
		preparation, provider.authority.RoleplayEarlierResponses,
	)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	optionalRecords := interleaveContextRecordGroups(
		currentRound, optionalConversation, rankableFrozen, historical,
	)
	set, err := buildContextCandidateSet(requiredRecords, optionalRecords)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	set.Replan = replan
	return set, nil
}

func (provider boundObjectiveContextProvider) roleplayContextAuthority() (
	roleplay.SimulationTurnAuthority,
	roleplay.NarrativeSimulationProjection,
	roleplay.SimulationResponderAuthority,
	error,
) {
	if provider.preparation == nil || provider.projection == nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			roleplay.SimulationResponderAuthority{},
			fmt.Errorf("roleplay context retrieval requires frozen simulation authority")
	}
	preparation := *provider.preparation
	projection := roleplay.CloneNarrativeSimulationProjection(*provider.projection)
	if err := preparation.Validate(); err != nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			roleplay.SimulationResponderAuthority{}, err
	}
	if err := projection.Validate(); err != nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			roleplay.SimulationResponderAuthority{}, err
	}
	responder, err := preparation.Responder(string(provider.authority.RoleplayViewpointCharacterID))
	if err != nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			roleplay.SimulationResponderAuthority{}, err
	}
	if !reflect.DeepEqual(responder.NarrativeProjection, projection) {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			roleplay.SimulationResponderAuthority{}, fmt.Errorf(
				"roleplay context projection differs from the selected responder authority",
			)
	}
	return preparation, projection, responder, nil
}

func roleplayContextRecordGroups(
	preparation roleplay.SimulationTurnAuthority,
	responder roleplay.SimulationResponderAuthority,
	conversation []queue.ContextSearchRecord,
	replan *assemblyline.ObjectiveReplanAuthority,
) (
	required []queue.ContextSearchRecord,
	optionalConversation []queue.ContextSearchRecord,
	rankableFrozen []queue.ContextSearchRecord,
	err error,
) {
	frozen, err := frozenRoleplayContextRecords(responder)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(frozen) < 2 || frozen[0].Namespace != "scene_state" ||
		frozen[1].Namespace != "scene_participants" {
		return nil, nil, nil, fmt.Errorf("roleplay context lost current scene authority")
	}
	required = append(required, replanContextRecords(replan)...)
	required = append(required, pendingTransitionContextRecords(preparation)...)
	required = append(required, frozen[:2]...)
	optionalConversation = append(optionalConversation, conversation...)
	return required, optionalConversation, frozen[2:], nil
}

func replanContextRecords(
	replan *assemblyline.ObjectiveReplanAuthority,
) []queue.ContextSearchRecord {
	if replan == nil {
		return nil
	}
	return []queue.ContextSearchRecord{{
		Namespace: "objective_replan",
		SourceID:  fmt.Sprintf("job-%d-generation-%d", replan.JobID, replan.Generation),
		Content:   replan.Feedback,
	}}
}

func interleaveContextRecordGroups(groups ...[]queue.ContextSearchRecord) []queue.ContextSearchRecord {
	total := 0
	maximum := 0
	for _, group := range groups {
		total += len(group)
		maximum = max(maximum, len(group))
	}
	records := make([]queue.ContextSearchRecord, 0, total)
	for index := 0; index < maximum; index++ {
		for _, group := range groups {
			if index < len(group) {
				records = append(records, group[index])
			}
		}
	}
	return records
}

func buildContextCandidateSet(
	requiredRecords []queue.ContextSearchRecord,
	optionalRecords []queue.ContextSearchRecord,
) (contextcompiler.CandidateSet, error) {
	type projectedRecord struct {
		namespace string
		content   string
		required  bool
		group     int
	}
	projected := make([]projectedRecord, 0, len(requiredRecords)+len(optionalRecords))
	seenRecordContent := make(map[string]struct{})
	requiredChunkCount := 0
	groupCount := 0
	appendRecords := func(records []queue.ContextSearchRecord, required bool) error {
		for _, record := range records {
			if strings.TrimSpace(record.Namespace) == "" || strings.TrimSpace(record.SourceID) == "" ||
				strings.TrimSpace(record.Content) == "" {
				return fmt.Errorf("context source %q has invalid exact authority", record.SourceID)
			}
			recordHash := assemblyline.ExactObjectiveContextSHA(record.Content)
			if _, duplicate := seenRecordContent[recordHash]; duplicate {
				continue
			}
			seenRecordContent[recordHash] = struct{}{}
			chunks := splitContextCandidateContent(record.Content)
			group := -1
			if !required && len(chunks) > 1 {
				group = groupCount
				groupCount++
			}
			for _, chunk := range chunks {
				if chunk == "" {
					return fmt.Errorf("context source %q has no usable exact content", record.SourceID)
				}
				projected = append(projected, projectedRecord{
					namespace: record.Namespace, content: chunk, required: required, group: group,
				})
				if required {
					requiredChunkCount++
				}
			}
		}
		return nil
	}
	if err := appendRecords(requiredRecords, true); err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	if err := appendRecords(optionalRecords, false); err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	required := make([]assemblyline.ContextCandidateAuthority, 0, len(requiredRecords))
	optional := make([]assemblyline.ContextCandidateAuthority, 0, len(optionalRecords))
	groups := make([]contextcompiler.OptionalSelectionGroup, groupCount)
	for _, record := range projected {
		candidateID := fmt.Sprintf("CTX_%d", len(required)+len(optional)+1)
		authority, err := assemblyline.NewContextCandidateAuthority(
			record.namespace, candidateID, record.content,
		)
		if err != nil {
			return contextcompiler.CandidateSet{}, err
		}
		if record.required {
			required = append(required, authority)
		} else {
			optional = append(optional, authority)
			if record.group >= 0 {
				groups[record.group].CandidateIDs = append(
					groups[record.group].CandidateIDs, candidateID,
				)
			}
		}
	}
	if len(required) != requiredChunkCount || len(required)+len(optional) != len(projected) {
		return contextcompiler.CandidateSet{}, fmt.Errorf("context candidate projection lost acquired authority")
	}
	return contextcompiler.CandidateSet{
		Required: required, Optional: optional, OptionalSelectionGroups: groups,
	}, nil
}

func splitContextCandidateContent(content string) []string {
	if content == "" {
		return []string{""}
	}
	maximum := assemblyline.MaxContextCandidateContentBytes
	if len(content) <= maximum {
		return []string{content}
	}
	const framingBudget = 64
	payloadMaximum := maximum - framingBudget
	rawChunks := make([]string, 0, (len(content)+payloadMaximum-1)/payloadMaximum)
	for len(content) > 0 {
		end := min(payloadMaximum, len(content))
		for end > 0 && end < len(content) && !utf8.RuneStart(content[end]) {
			end--
		}
		if end == 0 {
			return []string{""}
		}
		rawChunks = append(rawChunks, content[:end])
		content = content[end:]
	}
	chunks := make([]string, len(rawChunks))
	for index, raw := range rawChunks {
		chunks[index] = fmt.Sprintf(
			"Segment %d/%d:\n%s", index+1, len(rawChunks), raw,
		)
		if len(chunks[index]) > maximum {
			return []string{""}
		}
	}
	return chunks
}
