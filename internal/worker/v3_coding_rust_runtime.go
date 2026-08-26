package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func rustCommandLineRuntimeDocument() assemblyline.SourceDocument {
	const source = `use std::collections::BTreeMap;

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct TaskInput {
    pub arguments: Vec<String>,
    pub standard_input: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct TaskResult {
    pub output: String,
    pub error: String,
    pub exit_code: i32,
    pub state: BTreeMap<String, String>,
}

pub type CapabilityResults = BTreeMap<String, TaskResult>;`
	return assemblyline.SourceDocument{
		ID: "application_runtime", Path: "src/runtime.rs",
		Blocks: []assemblyline.SourceBlock{{ID: "runtime.api", Static: source, API: source}},
	}
}

func rustCommandLineLibraryDocument(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	order []string,
	modules map[string]string,
	moduleBlocks []assemblyline.SourceBlock,
	dependencies []string,
) assemblyline.SourceDocument {
	blocks := append([]assemblyline.SourceBlock(nil), moduleBlocks...)
	blocks = append(blocks, assemblyline.SourceBlock{
		ID:        "application.run",
		Static:    rustCommandLineApplicationSource(requirements, capabilities, order, modules),
		API:       "pub fn run_application(arguments: Vec<String>, standard_input: String) -> TaskResult",
		DependsOn: append([]string(nil), dependencies...),
	})
	return assemblyline.SourceDocument{
		ID: "application_library", Path: "src/lib.rs",
		Preamble: "pub mod runtime;\npub use runtime::{CapabilityResults, TaskInput, TaskResult};",
		Blocks:   blocks,
	}
}

func rustCommandLineApplicationSource(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	order []string,
	modules map[string]string,
) string {
	indices := make(map[string]int, len(requirements))
	for index, requirement := range requirements {
		indices[requirement.ID] = index + 1
	}
	var source strings.Builder
	source.WriteString("pub fn run_application(arguments: Vec<String>, standard_input: String) -> TaskResult {\n")
	source.WriteString("    let input = TaskInput { arguments, standard_input };\n")
	source.WriteString("    let mut results = CapabilityResults::new();\n")
	source.WriteString("    let mut combined = TaskResult::default();\n")
	for _, requirementID := range order {
		sequence := indices[requirementID]
		source.WriteString(fmt.Sprintf("    let mut direct_%03d = CapabilityResults::new();\n", sequence))
		for _, dependency := range capabilities[requirementID] {
			source.WriteString(fmt.Sprintf(
				"    let Some(dependency) = results.get(%s) else {\n", strconv.Quote(dependency.CapabilityID),
			))
			source.WriteString(fmt.Sprintf(
				"        return TaskResult { error: %s.to_string(), exit_code: 1, ..TaskResult::default() };\n",
				strconv.Quote("missing internal capability "+dependency.CapabilityID),
			))
			source.WriteString("    };\n")
			source.WriteString(fmt.Sprintf(
				"    direct_%03d.insert(%s.to_string(), dependency.clone());\n",
				sequence, strconv.Quote(dependency.CapabilityID),
			))
		}
		source.WriteString(fmt.Sprintf(
			"    let result_%03d = %s::feature_%03d(&input, &direct_%03d);\n",
			sequence, modules[requirementID], sequence, sequence,
		))
		source.WriteString(fmt.Sprintf(
			"    results.insert(%s.to_string(), result_%03d.clone());\n",
			strconv.Quote(genericApplicationCapabilityID(sequence)), sequence,
		))
		source.WriteString(fmt.Sprintf("    if !result_%03d.output.is_empty() {\n", sequence))
		source.WriteString("        if !combined.output.is_empty() { combined.output.push('\\n'); }\n")
		source.WriteString(fmt.Sprintf("        combined.output.push_str(&result_%03d.output);\n", sequence))
		source.WriteString("    }\n")
		source.WriteString(fmt.Sprintf(
			"    for (key, value) in &result_%03d.state { combined.state.insert(key.clone(), value.clone()); }\n",
			sequence,
		))
		source.WriteString(fmt.Sprintf(
			"    if result_%03d.exit_code != 0 { combined.exit_code = result_%03d.exit_code; }\n",
			sequence, sequence,
		))
		source.WriteString(fmt.Sprintf("    if !result_%03d.error.is_empty() {\n", sequence))
		source.WriteString(fmt.Sprintf("        combined.error = result_%03d.error;\n", sequence))
		source.WriteString("        if combined.exit_code == 0 { combined.exit_code = 1; }\n")
		source.WriteString("        return combined;\n    }\n")
	}
	source.WriteString("    combined\n}")
	return source.String()
}

func rustCommandLineMainDocument(crateName string) assemblyline.SourceDocument {
	const source = `fn main() {
    let mut standard_input = String::new();
    if let Err(error) = io::stdin().read_to_string(&mut standard_input) {
        eprintln!("read standard input: {error}");
        process::exit(1);
    }
    let result = run_application(env::args().skip(1).collect(), standard_input);
    if !result.output.is_empty() { println!("{}", result.output); }
    if !result.error.is_empty() { eprintln!("{}", result.error); }
    if result.exit_code != 0 { process::exit(result.exit_code); }
}`
	return assemblyline.SourceDocument{
		ID: "application_entrypoint", Path: "src/main.rs",
		Preamble: fmt.Sprintf(
			"use %s::run_application;\nuse std::{env, io::{self, Read}, process};", crateName,
		),
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.main", Static: source, API: "fn main()",
			DependsOn: []string{"application.run"},
		}},
	}
}
