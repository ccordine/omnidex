package worker

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func javaScriptRelativeModule(fromFile, toFile string) string {
	relative, err := filepath.Rel(filepath.Dir(filepath.FromSlash(fromFile)), filepath.FromSlash(toFile))
	if err != nil {
		panic(fmt.Sprintf("normalized JavaScript paths must be comparable: %v", err))
	}
	relative = filepath.ToSlash(relative)
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	return relative
}

func javaScriptCommandLineRuntimeDocument(
	profile directCodingProjectVersionProfile,
) (assemblyline.SourceDocument, error) {
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	guard, err := javaScriptNodeRuntimeGuard(node)
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	const source = `export function normalizeTaskResult(value) {
  if (value === null || typeof value !== 'object' || Array.isArray(value) || Object.getPrototypeOf(value) !== Object.prototype) {
    throw new TypeError('feature result must be one plain object');
  }
  const required = ['error', 'exitCode', 'output', 'state'];
  const keys = Object.keys(value).sort();
  if (keys.length !== required.length || keys.some((key, index) => key !== required[index])) {
    throw new TypeError('feature result requires exactly output, error, exitCode, and state');
  }
  if (typeof value.output !== 'string' || typeof value.error !== 'string' ||
      !Number.isSafeInteger(value.exitCode) || value.state === null ||
      typeof value.state !== 'object' || Array.isArray(value.state) ||
      Object.getPrototypeOf(value.state) !== Object.prototype) {
    throw new TypeError('feature result fields require string, string, safe integer, and plain object types');
  }
  if (value.output !== '' && value.error !== '') {
    throw new TypeError('feature result cannot contain both output and error');
  }
  return { output: value.output, error: value.error, exitCode: value.exitCode, state: { ...value.state } };
}`
	return assemblyline.SourceDocument{
		ID: "application_runtime", Path: "runtime.mjs",
		Blocks: []assemblyline.SourceBlock{{
			ID: "runtime.api", Static: guard + "\n\n" + source,
			API: "export function normalizeTaskResult(value)",
		}},
	}, nil
}

func javaScriptCommandLineApplicationDocument(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	order []string,
	coverage assemblyline.ApplicationFileCoveragePlan,
	contexts map[string]assemblyline.ApplicationTaskContext,
	dependencies []string,
) assemblyline.SourceDocument {
	imports := []string{"import { pathToFileURL } from 'node:url';", "import { normalizeTaskResult } from './runtime.mjs';"}
	for index, requirement := range requirements {
		context := contexts[requirement.ID]
		pair, err := directCodingTaskSinglePair(coverage, context.Task.TaskID)
		if err != nil {
			panic(fmt.Sprintf("validated JavaScript coverage changed: %v", err))
		}
		name := fmt.Sprintf("feature%03d", index+1)
		imports = append(imports, fmt.Sprintf("import { %s } from %s;", name,
			strconv.Quote(javaScriptRelativeModule("main.mjs", pair.ImplementationPath))))
	}
	return assemblyline.SourceDocument{
		ID: "application_entrypoint", Path: "main.mjs", Preamble: strings.Join(imports, "\n"),
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.run", Static: javaScriptCommandLineApplicationSource(requirements, capabilities, order),
			API:       "function runApplication(arguments, standardInput)",
			DependsOn: append([]string(nil), dependencies...),
		}},
	}
}

func javaScriptCommandLineApplicationSource(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	order []string,
) string {
	indices := make(map[string]int, len(requirements))
	for index, requirement := range requirements {
		indices[requirement.ID] = index + 1
	}
	var source strings.Builder
	source.WriteString("export function runApplication(arguments, standardInput) {\n")
	source.WriteString("  const input = { arguments: [...arguments], standardInput };\n")
	source.WriteString("  const results = {};\n  const combined = { output: '', error: '', exitCode: 0, state: {} };\n")
	for _, requirementID := range order {
		sequence := indices[requirementID]
		source.WriteString(fmt.Sprintf("  const direct%03d = {\n", sequence))
		for _, dependency := range capabilities[requirementID] {
			source.WriteString(fmt.Sprintf(
				"    [%s]: results[%s],\n",
				strconv.Quote(dependency.CapabilityID), strconv.Quote(dependency.CapabilityID),
			))
		}
		source.WriteString("  };\n")
		source.WriteString(fmt.Sprintf("  const result%03d = normalizeTaskResult(feature%03d(input, direct%03d));\n", sequence, sequence, sequence))
		source.WriteString(fmt.Sprintf("  results[%s] = result%03d;\n", strconv.Quote(genericApplicationCapabilityID(sequence)), sequence))
		source.WriteString(fmt.Sprintf("  if (result%03d.output) combined.output += (combined.output ? '\\n' : '') + result%03d.output;\n", sequence, sequence))
		source.WriteString(fmt.Sprintf("  Object.assign(combined.state, result%03d.state);\n", sequence))
		source.WriteString(fmt.Sprintf("  if (result%03d.exitCode) combined.exitCode = result%03d.exitCode;\n", sequence, sequence))
		source.WriteString(fmt.Sprintf("  if (result%03d.error) { combined.error = result%03d.error; return combined; }\n", sequence, sequence))
	}
	source.WriteString("  return combined;\n}\n\n")
	source.WriteString(`if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  let standardInput = '';
  process.stdin.setEncoding('utf8');
  for await (const chunk of process.stdin) standardInput += chunk;
  const result = runApplication(process.argv.slice(2), standardInput);
  if (result.output) process.stdout.write(result.output + '\n');
  if (result.error) process.stderr.write(result.error + '\n');
  process.exitCode = result.exitCode;
}`)
	return source.String()
}
