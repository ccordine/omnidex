package worker

import "fmt"

func cloneValidatedDirectCodingStaticFiles(
	stack directCodingProjectStack,
	files []directCodingFileTask,
) ([]directCodingFileTask, error) {
	if stack.ID == "" {
		return nil, fmt.Errorf("static-file projection requires one registered project stack")
	}
	projected := make([]directCodingFileTask, len(files))
	seen := make(map[string]struct{}, len(files))
	for index, file := range files {
		if _, err := requireExactDirectCodingPath(file.Path); err != nil {
			return nil, fmt.Errorf("project static file %d: %w", index+1, err)
		}
		if _, duplicate := seen[file.Path]; duplicate {
			return nil, fmt.Errorf("project static files repeat path %q", file.Path)
		}
		seen[file.Path] = struct{}{}
		if file.Mode == 0 || file.Mode&^uint32(0o777) != 0 {
			return nil, fmt.Errorf("project static file %q has invalid permission mode", file.Path)
		}
		if file.MoveFrom != "" {
			return nil, fmt.Errorf("project static file %q cannot carry move authority", file.Path)
		}
		if err := validateDirectCodingStackArtifactSource(stack, file.Path, file.Content); err != nil {
			return nil, fmt.Errorf("validate project static file %q: %w", file.Path, err)
		}
		projected[index] = file
		projected[index].Content = append([]byte{}, file.Content...)
	}
	return projected, nil
}
