package cognitiongauntlet

import (
	"bytes"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func TestPostgresPreEpisodeBrainBootstrapFailureReplayIsExactAndDeterministic(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	_, bundle, request := publicFailureFixture(t, ctx, pool, repository, hostStore)
	if _, err := ExportPreEpisodeBrainBootstrapFailureReplay(
		ctx, repository, bundle, request.Attempt,
	); err == nil {
		t.Fatal("pre-episode replay accepted a missing failure record")
	}
	runPublicProviderIdentityFailure(t, ctx, bundle, request, 1)
	first, err := ExportPreEpisodeBrainBootstrapFailureReplay(
		ctx, repository, bundle, request.Attempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExportPreEpisodeBrainBootstrapFailureReplay(
		ctx, repository, bundle, request.Attempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 || !bytes.Equal(first.Bytes, second.Bytes) {
		t.Fatal("identical pre-episode failure did not produce one deterministic replay")
	}
	verified, err := cognitionreplay.VerifyBase(first.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	manifest := verified.Manifest()
	terminal, ok := manifest.TerminalAuthority.PreEpisodeBrainBootstrapFailure()
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || manifest.Schema != cognitionreplay.BaseSchemaV2 ||
		manifest.EventCount != 6 || manifest.CheckpointCount != 2 ||
		terminal.RecordID == "" || terminal.RequestedEpisodeID != string(episode.ID) {
		t.Fatalf("pre-episode replay manifest=%+v terminal=%+v", manifest, terminal)
	}
	var episodes int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM cognition_episodes WHERE episode_id=$1`, episode.ID,
	).Scan(&episodes); err != nil {
		t.Fatal(err)
	}
	if episodes != 0 {
		t.Fatalf("pre-episode replay invented %d episode rows", episodes)
	}
}

func TestPostgresPreEpisodeReplayRejectsProviderProcessFailure(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	_, bundle, request := publicFailureFixture(t, ctx, pool, repository, hostStore)
	runPublicProviderIdentityFailure(t, ctx, bundle, request, 2)
	if _, err := ExportPreEpisodeBrainBootstrapFailureReplay(
		ctx, repository, bundle, request.Attempt,
	); err == nil {
		t.Fatal("Brain-bootstrap replay accepted a provider-process failure")
	}
}
