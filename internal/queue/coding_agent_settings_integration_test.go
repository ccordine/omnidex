package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/projectroot"
)

func TestCodingAgentSettingsPersistAndFreezeOnlyNewJobs(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	startup := codingAgentTestConfiguration()
	repository.WithCodingAgents(startup)
	ctx := context.Background()
	configuration, source, err := repository.LoadCodingAgentConfiguration(ctx)
	if err != nil || source != "startup" || configuration != startup {
		t.Fatalf("absent settings did not use declared startup values: %#v %q %v", configuration, source, err)
	}
	saved := startup
	saved.Executor.Model = "saved-executor"
	saved.Executor.Effort = "medium"
	saved.Executor.ServiceTier = "fast"
	saved.Reviewer.Effort = "high"
	saved.Reviewer.ServiceTier = "default"
	if err := repository.StoreCodingAgentConfiguration(ctx, saved); err != nil {
		t.Fatal(err)
	}
	configuration, source, err = repository.LoadCodingAgentConfiguration(ctx)
	if err != nil || source != "saved" || configuration != saved {
		t.Fatalf("persisted routes changed: %#v %q %v", configuration, source, err)
	}
	job, err := repository.EnqueueCodingJobMode(ctx, "Use the saved executor.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil || frozen != saved {
		t.Fatalf("new coding job did not freeze saved routes: %#v %v", frozen, err)
	}
	if busy, err := repository.CodingAgentsBusy(ctx); err != nil || !busy {
		t.Fatalf("pending Agent job not reported busy: %v %v", busy, err)
	}
	root := t.TempDir()
	directory, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := projectroot.ClientWorkspaceIdentity("11111111111111111111111111111111", directory)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.EnsureCLIChatSessionChannel(ctx, root, identity)
	if err != nil {
		t.Fatal(err)
	}
	_, channelJob, err := repository.EnqueueChannelTurnWithCodingMode(ctx, channel.ID, "Use saved reviewer routes.", "", codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	channelFrozen, err := codingagent.ConfigurationFromMetadata(channelJob.Metadata)
	if err != nil || channelFrozen != saved {
		t.Fatalf("channel job did not freeze saved routes: %#v %v", channelFrozen, err)
	}
	changed := saved
	changed.Executor.Model = "later-executor"
	if err := repository.StoreCodingAgentConfiguration(ctx, changed); err != nil {
		t.Fatal(err)
	}
	retained, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := codingagent.ConfigurationFromMetadata(retained.Job.Metadata)
	if err != nil || existing != saved {
		t.Fatalf("saved settings mutated an existing job: %#v %v", existing, err)
	}
	ledgerJob, err := repository.EnqueueCodingJob(ctx, "Coding uses the task ledger.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledgerConfiguration, err := codingagent.ConfigurationFromMetadata(ledgerJob.Metadata)
	if err != nil || ledgerConfiguration != changed {
		t.Fatalf("coding ledger did not freeze the task-ledger configuration: %#v %v", ledgerConfiguration, err)
	}
	// A present malformed value is a failure, never a reason to use startup defaults.
	if _, err := pool.Exec(ctx, `UPDATE coding_agent_settings SET configuration=jsonb_set(configuration,'{executor,model}','""'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.LoadCodingAgentConfiguration(ctx); err == nil {
		t.Fatal("invalid saved settings silently used startup values")
	}
}

func TestCodingAgentSettingsRejectNonAgentAndBusyExcludesTerminalJobs(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	if err := repository.StoreCodingAgentConfiguration(ctx, codingagent.Configuration{}); err == nil {
		t.Fatal("settings accepted a non-Agent configuration")
	}
	job, err := repository.EnqueueCodingJobMode(ctx, "Cancel the fixture.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	if _, err := repository.CancelJob(ctx, CancelJobCommand{OperationID: codingPlanOperationID(t, "cancel-settings-fixture", job.ID), JobID: job.ID, Reason: "Fixture complete.", WorkspaceRoot: root, WorkspaceIdentity: identity}); err != nil {
		t.Fatal(err)
	}
	if busy, err := repository.CodingAgentsBusy(ctx); err != nil || busy {
		t.Fatalf("terminal fixture blocked settings: %v %v", busy, err)
	}
}

func TestCodingJobRejectsMissingTaskLedgerRoutes(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingagent.Configuration{})
	if _, err := repository.EnqueueCodingJob(context.Background(), "Reject missing routes.", t.TempDir()); err == nil {
		t.Fatal("coding job silently entered an alternate pipeline without task-ledger routes")
	}
}

func TestCodingAgentBusyTracksOnlyTaskLedgerCoding(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	assertBusy := func(want bool) {
		t.Helper()
		busy, err := repository.CodingAgentsBusy(ctx)
		if err != nil || busy != want {
			t.Fatalf("busy=%v want=%v error=%v", busy, want, err)
		}
	}
	assertBusy(false)
	if _, err := repository.EnqueueCodingJobMode(ctx, "Reject removed coding mode.", t.TempDir(), "normal"); err == nil {
		t.Fatal("removed normal coding mode was admitted")
	}
	job, err := repository.EnqueueCodingJob(ctx, "Check the installed provider idle guard.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	assertBusy(true)
	root, identity := codingPlanWorkspaceAuthority(t, job)
	if _, err := repository.CancelJob(ctx, CancelJobCommand{OperationID: codingPlanOperationID(t, "cancel-provider-idle-fixture", job.ID), JobID: job.ID, Reason: "Fixture complete.", WorkspaceRoot: root, WorkspaceIdentity: identity}); err != nil {
		t.Fatal(err)
	}
	assertBusy(false)
}
