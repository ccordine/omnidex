package workspace

import (
	"fmt"
	"os"
)

func verifyWorkspacePathAbsent(root *authoritativeWorkspaceRoot, relative string) error {
	if root == nil {
		return fmt.Errorf("verify absent workspace path requires an open root")
	}
	_, err := root.Lstat(relative)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("workspace path still exists")
}

func verifyAndRecordWorkspaceDeletion(
	root *authoritativeWorkspaceRoot,
	result *ReconciliationResult,
	observer VerifiedChangeObserver,
	relative string,
) error {
	if err := verifyWorkspacePathAbsent(root, relative); err != nil {
		return err
	}
	recordVerifiedChange(result, observer, Change{Path: relative, Kind: ChangeDelete})
	return nil
}

func recordVerifiedChange(
	result *ReconciliationResult,
	observer VerifiedChangeObserver,
	change Change,
) {
	result.Changes = append(result.Changes, change)
	if observer != nil {
		observer(change)
	}
}
