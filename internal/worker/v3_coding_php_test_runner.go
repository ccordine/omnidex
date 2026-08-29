package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func phpServiceTestRunnerDocument(
	bindings []phpServiceFeatureBinding,
	dependencies []string,
	storage directCodingServiceStoragePlan,
) assemblyline.SourceDocument {
	preamble := []string{"<?php", "declare(strict_types=1);"}
	for _, binding := range bindings {
		preamble = append(preamble, fmt.Sprintf(
			"require_once __DIR__ . '/Feature%sTest.php';", binding.FeatureNumber,
		))
	}
	preamble = append(preamble, "require_once __DIR__ . '/../public/index.php';")
	dependencies = append(append([]string(nil), dependencies...), "application.http")
	if storage.RequiresPostgreSQL() {
		dependencies = append(dependencies, "runtime.state")
	}
	return assemblyline.SourceDocument{
		ID: "application_test_runner", Path: "tests/TestRunner.php", AdapterID: phpSourceAdapterID,
		Preamble: strings.Join(preamble, "\n"),
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.verify", Static: phpServiceTestRunnerSource(bindings, storage),
			API:       "run every accepted PHP HTTP verification and fail with all observed errors",
			DependsOn: append([]string(nil), dependencies...),
		}},
	}
}

func phpServiceTestRunnerSource(
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
	source.WriteString(phpServiceHTTPSmokeSource(bindings, stateful))
	source.WriteString("if ($failures !== []) {\n")
	source.WriteString("    fwrite(STDERR, implode(PHP_EOL, $failures) . PHP_EOL);\n")
	source.WriteString("    exit(1);\n")
	source.WriteString("}\n")
	source.WriteString("fwrite(STDOUT, 'PHP HTTP verification passed.' . PHP_EOL);\n")
	return source.String()
}
