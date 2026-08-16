package assemblyline

import "fmt"

// AcceptanceFailureObservation is one source-free public operation retained
// from the code-owned acceptance inventory.
type AcceptanceFailureObservation struct {
	Operations []string                       `json:"operations"`
	Literals   []AcceptanceObservationLiteral `json:"literals"`
}

// AcceptanceFailureEvidence binds a failing public observation to the nearest
// preceding public interaction without exposing acceptance source.
type AcceptanceFailureEvidence struct {
	Failure              string                         `json:"failure"`
	RequiredObservation  []AcceptanceFailureObservation `json:"required_observation"`
	PrecedingInteraction []AcceptanceFailureObservation `json:"preceding_interaction,omitempty"`
}

func ResolveTypeScriptAcceptanceFailureEvidence(
	source string,
	tsx bool,
	line int,
	column int,
) (AcceptanceFailureEvidence, error) {
	var zero AcceptanceFailureEvidence
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, tsx)
	if err != nil {
		return zero, err
	}
	siteID, mapped, err := ResolveTypeScriptAcceptanceObservationSite(source, tsx, line, column)
	if err != nil {
		return zero, err
	}
	if !mapped {
		return zero, fmt.Errorf("acceptance failure has no rooted public observation")
	}
	statementOrder := make(map[string]int, len(inventory.Statements))
	for index, statement := range inventory.Statements {
		statementOrder[statement.ID] = index
	}
	failingStatement := ""
	for _, site := range inventory.Sites {
		if site.ID == siteID {
			failingStatement = site.StatementID
			break
		}
	}
	failingIndex, exists := statementOrder[failingStatement]
	if !exists {
		return zero, fmt.Errorf("acceptance failure site %s has no current statement", siteID)
	}
	evidence := AcceptanceFailureEvidence{
		Failure:             "required_observation_absent",
		RequiredObservation: acceptanceFailureStatementObservations(inventory, failingStatement),
	}
	if len(evidence.RequiredObservation) == 0 {
		return zero, fmt.Errorf("acceptance failure statement %s has no public observation", failingStatement)
	}
	for index := failingIndex - 1; index >= 0; index-- {
		statementID := inventory.Statements[index].ID
		observations := acceptanceFailureStatementObservations(inventory, statementID)
		if acceptanceFailureObservationsContainInteraction(observations) {
			evidence.PrecedingInteraction = observations
			break
		}
	}
	return evidence, nil
}

func acceptanceFailureStatementObservations(
	inventory AcceptanceObservationInventory,
	statementID string,
) []AcceptanceFailureObservation {
	result := make([]AcceptanceFailureObservation, 0, 2)
	for _, site := range inventory.Sites {
		if site.StatementID != statementID || len(site.Operations) == 0 ||
			stringInSet("untrusted_call", site.Operations) || acceptanceFailurePureHarnessSite(site) {
			continue
		}
		result = append(result, AcceptanceFailureObservation{
			Operations: append([]string(nil), site.Operations...),
			Literals:   append([]AcceptanceObservationLiteral(nil), site.Literals...),
		})
	}
	return result
}

func acceptanceFailurePureHarnessSite(site AcceptanceObservationSite) bool {
	return len(site.Operations) == 1 && site.Operations[0] == "harness_call:waitFor"
}

func acceptanceFailureObservationsContainInteraction(
	observations []AcceptanceFailureObservation,
) bool {
	for _, observation := range observations {
		for _, operation := range observation.Operations {
			if len(operation) > len("fire_event:") && operation[:len("fire_event:")] == "fire_event:" {
				return true
			}
		}
	}
	return false
}
