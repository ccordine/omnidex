package worker

import (
	"context"
	"errors"

	"github.com/gryph/omnidex/internal/projectroot"
)

func validateDirectCodingAssemblyAtRoot(root string, assembly directCodingAssembly) (resultErr error) {
	reader, err := projectroot.OpenDirectory(root)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, reader.Close()) }()
	return validateDirectCodingAssembly(context.Background(), reader, assembly)
}

func validateDirectCodingTypeScriptGreenfieldProgramRoot(root string, program directCodingProgram) (resultErr error) {
	reader, err := projectroot.OpenDirectory(root)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, reader.Close()) }()
	return validateDirectCodingTypeScriptGreenfieldProgram(context.Background(), reader, program)
}

func snapshotDirectCodingTargetTreeOccupationAtRoot(root string, stack directCodingProjectStack) (occupation directCodingTargetTreeOccupation, resultErr error) {
	reader, err := projectroot.OpenDirectory(root)
	if err != nil {
		return occupation, err
	}
	defer func() { resultErr = errors.Join(resultErr, reader.Close()) }()
	return snapshotDirectCodingTargetTreeOccupation(context.Background(), reader, stack)
}
