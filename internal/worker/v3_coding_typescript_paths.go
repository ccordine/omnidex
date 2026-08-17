package worker

import (
	"fmt"
	"path/filepath"
	"strings"
)

func typeScriptRelativeModule(fromFile, toFile string) string {
	relative, err := filepath.Rel(filepath.Dir(filepath.FromSlash(fromFile)), filepath.FromSlash(toFile))
	if err != nil {
		panic(fmt.Sprintf("normalized TypeScript paths must be comparable: %v", err))
	}
	relative = filepath.ToSlash(relative)
	relative = strings.TrimSuffix(relative, filepath.Ext(relative))
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	return relative
}
