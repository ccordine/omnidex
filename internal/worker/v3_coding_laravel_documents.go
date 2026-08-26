package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func laravelServiceDocuments(
	base []assemblyline.SourceDocument,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
	endpoints directCodingServiceEndpointPlan,
	state directCodingServiceStatePlan,
	storage directCodingServiceStoragePlan,
) ([]assemblyline.SourceDocument, error) {
	bindings, byRequirement, err := phpServiceFeatureBindings(
		specification, workload, coverage, endpoints,
	)
	if err != nil {
		return nil, err
	}
	routeBlocks, err := phpServiceRouteBlocks(bindings)
	if err != nil {
		return nil, err
	}
	byTask := make(map[string]phpServiceFeatureBinding, len(bindings))
	for _, binding := range bindings {
		byTask[binding.TaskID] = binding
	}
	documents := make([]assemblyline.SourceDocument, 0, len(base))
	for _, document := range base {
		switch document.Path {
		case "src/Runtime.php":
			documents = append(documents, laravelRuntimeDocument(
				phpServiceHasHTMLResponse(endpoints), storage.RequiresPostgreSQL(), routeBlocks,
			))
			continue
		case "public/index.php", "tests/TestRunner.php":
			continue
		}
		if phpServiceVerificationPath.MatchString(document.Path) {
			if len(document.Blocks) == 0 {
				return nil, fmt.Errorf("Laravel verification %s has no bounded blocks", document.Path)
			}
			binding, exists := byTask[document.Blocks[0].TaskID]
			if !exists {
				return nil, fmt.Errorf("Laravel verification %s has no task binding", document.Path)
			}
			document.Preamble = laravelFeatureVerificationPreamble(binding.Implementation)
		}
		documents = append(documents, document)
	}
	router, err := laravelRouterDocument(
		bindings, specification.Requirements, capabilities, byRequirement, state,
	)
	if err != nil {
		return nil, err
	}
	documents = append(documents, router, laravelTestRunnerDocument(bindings, storage))
	return documents, nil
}

func laravelRuntimeDocument(
	hasHTML, hasState bool,
	routeBlocks []assemblyline.SourceBlock,
) assemblyline.SourceDocument {
	blocks := []assemblyline.SourceBlock{
		{ID: "runtime.task_input", Static: phpServiceTaskInputSource(), API: phpServiceTaskInputAPI()},
		{ID: "runtime.task_result", Static: phpServiceTaskResultSource(), API: phpServiceTaskResultAPI()},
	}
	if hasState {
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: "runtime.state", Static: laravelStateRuntimeSource(),
			API: laravelStateRuntimeAPI(),
		})
	}
	if hasHTML {
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: "runtime.html", Static: phpServiceHTMLRuntimeSource(),
			API: phpServiceHTMLRuntimeAPI(), DependsOn: []string{"runtime.task_result"},
		})
	}
	blocks = append(blocks, routeBlocks...)
	blocks = append(blocks,
		assemblyline.SourceBlock{
			ID: "runtime.assertions", Static: phpServiceAssertionsSource(),
			API: phpServiceAssertionsAPI(), DependsOn: []string{"runtime.task_result"},
		},
		assemblyline.SourceBlock{
			ID: "runtime.laravel.http", Static: laravelHTTPRuntimeSource(),
			API:       "code-owned Laravel request decoding and response emission",
			DependsOn: []string{"runtime.task_input", "runtime.task_result", "runtime.assertions"},
		},
	)
	return assemblyline.SourceDocument{
		ID: "application_runtime", Path: "src/Runtime.php", AdapterID: phpSourceAdapterID,
		Preamble: "<?php\ndeclare(strict_types=1);", Blocks: blocks,
	}
}

func laravelFeatureVerificationPreamble(implementationPath string) string {
	base := strings.TrimPrefix(implementationPath, "src/")
	return "<?php\ndeclare(strict_types=1);\n\n" +
		"require_once __DIR__ . '/LaravelBootstrap.php';\n" +
		"require_once __DIR__ . '/../src/Runtime.php';\n" +
		"require_once __DIR__ . '/../src/" + base + "';"
}

func laravelTestRunnerDocument(
	bindings []phpServiceFeatureBinding,
	storage directCodingServiceStoragePlan,
) assemblyline.SourceDocument {
	preamble := []string{
		"<?php", "declare(strict_types=1);",
		"require_once __DIR__ . '/LaravelBootstrap.php';",
	}
	dependencies := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		preamble = append(preamble, fmt.Sprintf(
			"require_once __DIR__ . '/Feature%sTest.php';", binding.FeatureNumber,
		))
		dependencies = append(dependencies, binding.AcceptanceID)
	}
	return assemblyline.SourceDocument{
		ID: "application_test_runner", Path: "tests/TestRunner.php", AdapterID: phpSourceAdapterID,
		Preamble: strings.Join(preamble, "\n"),
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.verify", Static: laravelTestRunnerSource(bindings, storage),
			API:       "run every accepted Laravel feature verification and fail with all observed errors",
			DependsOn: dependencies,
		}},
	}
}

func laravelTestRunnerSource(
	bindings []phpServiceFeatureBinding,
	storage directCodingServiceStoragePlan,
) string {
	var source strings.Builder
	stateful := storage.RequiresPostgreSQL()
	if stateful {
		source.WriteString(phpServiceStateResetFunctionSource(storage.Namespace))
		source.WriteString("\n\n")
	}
	source.WriteString("$failures = [];\n")
	for _, binding := range bindings {
		source.WriteString("try {\n")
		if stateful {
			source.WriteString("    " + phpServiceStateResetFunctionName + "();\n")
		}
		source.WriteString("    " + binding.VerificationName + "();\n")
		source.WriteString("} catch (Throwable $failure) {\n")
		source.WriteString(fmt.Sprintf(
			"    $failures[] = %s . ': ' . $failure->getMessage();\n",
			phpSingleQuoted(binding.VerificationName),
		))
		source.WriteString("}\n")
		if stateful {
			source.WriteString("try {\n")
			source.WriteString("    " + phpServiceStateResetFunctionName + "();\n")
			source.WriteString("} catch (Throwable $failure) {\n")
			source.WriteString(fmt.Sprintf(
				"    $failures[] = %s . ': ' . $failure->getMessage();\n",
				phpSingleQuoted(binding.VerificationName+" state isolation"),
			))
			source.WriteString("}\n")
		}
	}
	source.WriteString("if ($failures !== []) {\n")
	source.WriteString("    fwrite(STDERR, implode(PHP_EOL, $failures) . PHP_EOL);\n")
	source.WriteString("    exit(1);\n")
	source.WriteString("}\n")
	source.WriteString("fwrite(STDOUT, 'Laravel feature verification passed.' . PHP_EOL);\n")
	return source.String()
}
