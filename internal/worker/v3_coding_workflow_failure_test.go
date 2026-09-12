package worker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type failingAssemblyWorkflowDriver struct {
	root         string
	assembly     directCodingAssembly
	assemblyErr  error
	prepareCalls int
	applyCalls   int
}

func (driver *failingAssemblyWorkflowDriver) Phase(directCodingPhase, string) {}

func (driver *failingAssemblyWorkflowDriver) Assemble() (directCodingAssembly, error) {
	return driver.assembly, driver.assemblyErr
}

func (driver *failingAssemblyWorkflowDriver) PrepareAssembly(
	directCodingAssembly,
) (*directCodingPreparedMutation, error) {
	driver.prepareCalls++
	return &directCodingPreparedMutation{}, nil
}

func (driver *failingAssemblyWorkflowDriver) ApplyAndVerify(
	*directCodingPreparedMutation,
) error {
	driver.applyCalls++
	if driver.root != "" {
		return os.WriteFile(filepath.Join(driver.root, "package.json"), []byte("partial"), 0o600)
	}
	return nil
}

func (driver *failingAssemblyWorkflowDriver) Complete() string { return "unexpected" }

func TestDirectCodingWorkflowNeverAppliesAssemblyReturnedWithError(t *testing.T) {
	driver := &failingAssemblyWorkflowDriver{
		assembly: directCodingAssembly{
			Files: []directCodingFileTask{{
				Path: "created.txt", Content: []byte("partial"), Mode: 0o644,
			}},
			DeletePaths: []string{"deleted.txt"},
		},
		assemblyErr: errors.New("bounded construction failed"),
	}
	if _, err := runDirectCodingWorkflow(driver); err == nil {
		t.Fatal("failed assembly unexpectedly completed")
	}
	if driver.prepareCalls != 0 || driver.applyCalls != 0 {
		t.Fatalf(
			"failed assembly reached mutation boundary: prepare=%d apply=%d",
			driver.prepareCalls, driver.applyCalls,
		)
	}
}

func TestBrowserConstructionFailureLeavesLiteralWorkspaceReusable(t *testing.T) {
	root := t.TempDir()
	userPath := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(userPath, []byte("user-owned"), 0o600); err != nil {
		t.Fatalf("write initial workspace: %v", err)
	}
	program := testTypeScriptBrowserProgramAtRoot(
		t,
		"A neutral browser utility",
		"Expose one observable state.",
		root,
	)
	driver := &failingAssemblyWorkflowDriver{
		root: root,
		assembly: directCodingAssembly{
			Files:       append([]directCodingFileTask(nil), program.StaticFiles...),
			DeletePaths: []string{"notes.txt"},
		},
		assemblyErr: errors.New("task-local source generation failed"),
	}
	if _, err := runDirectCodingWorkflow(driver); err == nil {
		t.Fatal("browser construction failure unexpectedly completed")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("inspect workspace after failure: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "notes.txt" || driver.prepareCalls != 0 || driver.applyCalls != 0 {
		t.Fatalf(
			"failed browser construction changed cwd: entries=%v prepare=%d apply=%d",
			entries, driver.prepareCalls, driver.applyCalls,
		)
	}
	if err := validateDirectCodingTypeScriptGreenfieldProgramRoot(root, program); err != nil {
		t.Fatalf("fresh redirected attempt was blocked by partial output: %v", err)
	}
	content, err := os.ReadFile(userPath)
	if err != nil || string(content) != "user-owned" {
		t.Fatalf("user file changed after failed construction: %q, %v", content, err)
	}
}
