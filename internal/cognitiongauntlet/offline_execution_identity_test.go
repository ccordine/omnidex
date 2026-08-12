package cognitiongauntlet

import "testing"

func TestSealedEpisodeBindsExactExecutionIdentity(t *testing.T) {
	base := validOfflineOutputConfig(t)
	authority := base.executionAuthority()
	manifest := EpisodeManifest{
		OmnidexCommit:           authority.OmnidexCommit,
		LedgerSchemaVersion:     authority.LedgerSchemaVersion,
		WorkingSetPolicyVersion: authority.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: authority.ProjectionPolicyVersion,
	}
	if err := validateEpisodeExecutionIdentity(manifest, authority); err != nil {
		t.Fatalf("exact execution identity was rejected: %v", err)
	}
	for name, mutate := range map[string]func(*EpisodeManifest){
		"Omnidex commit": func(value *EpisodeManifest) {
			value.OmnidexCommit = "0123456789abcdef0123456789abcdef01234567"
		},
		"ledger schema": func(value *EpisodeManifest) {
			value.LedgerSchemaVersion += ".changed"
		},
		"Working Set policy": func(value *EpisodeManifest) {
			value.WorkingSetPolicyVersion += ".changed"
		},
		"projection policy": func(value *EpisodeManifest) {
			value.ProjectionPolicyVersion += ".changed"
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := manifest
			mutate(&changed)
			if validateEpisodeExecutionIdentity(changed, authority) == nil {
				t.Fatal("changed execution identity was accepted")
			}
		})
	}
}
