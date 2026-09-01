package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericBrowserAppDocument(
	specification assemblyline.ApplicationSpecification,
	contexts map[string]assemblyline.ApplicationTaskContext,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceDocument, error) {
	imports := []string{
		"import { useMemo } from 'react';",
		"import type { ReactElement } from 'react';",
		"import { createApplicationRuntime, createFeatureRuntime } from './runtime';",
	}
	dependencies := []string{"runtime.factory"}
	for index := range specification.Requirements {
		requirement := specification.Requirements[index]
		context, exists := contexts[requirement.ID]
		if !exists {
			return assemblyline.SourceDocument{}, fmt.Errorf(
				"application workload omits requirement %s", requirement.ID,
			)
		}
		pair, err := directCodingTaskSinglePair(coverage, context.Task.TaskID)
		if err != nil {
			return assemblyline.SourceDocument{}, err
		}
		implementationPath := pair.ImplementationPath
		name := fmt.Sprintf("Feature%03d", index+1)
		imports = append(imports, fmt.Sprintf("import { %s } from '%s';", name, typeScriptRelativeModule("src/App.tsx", implementationPath)))
		dependencies = append(dependencies, fmt.Sprintf("feature.%03d", index+1))
	}
	return assemblyline.SourceDocument{
		ID: "application_shell", Path: "src/App.tsx", Preamble: strings.Join(imports, "\n"),
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.render", Static: genericBrowserAppSource(specification),
			API: "function App(): ReactElement", DependsOn: dependencies,
		}},
	}, nil
}

func genericBrowserAppSource(specification assemblyline.ApplicationSpecification) string {
	var body strings.Builder
	body.WriteString("export function App(): ReactElement {\n")
	body.WriteString("  const runtime = useMemo(() => createApplicationRuntime(), []);\n")
	body.WriteString("  const features = useMemo(() => ({\n")
	for index := range specification.Requirements {
		sequence := index + 1
		body.WriteString(fmt.Sprintf(
			"    feature%03d: createFeatureRuntime(runtime, %s),\n",
			sequence, strconv.Quote(genericApplicationCapabilityID(sequence)),
		))
	}
	body.WriteString("  }), [runtime]);\n")
	body.WriteString("  return (\n")
	body.WriteString("    <main className=\"isolate mx-auto grid gap-6 p-6\">\n")
	body.WriteString("      <header>\n")
	body.WriteString("        <h1>" + escapeTypeScriptJSXText(specification.ProductQuote) + "</h1>\n")
	body.WriteString("      </header>\n")
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		body.WriteString(fmt.Sprintf(
			"      <section aria-label={%s}>\n", strconv.Quote(requirement.SourceQuote),
		))
		body.WriteString(fmt.Sprintf("        <Feature%03d runtime={features.feature%03d} />\n", sequence, sequence))
		body.WriteString("      </section>\n")
	}
	body.WriteString("    </main>\n")
	body.WriteString("  );\n")
	body.WriteString("}")
	return body.String()
}

func escapeTypeScriptJSXText(value string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", "{", "&#123;", "}", "&#125;",
	).Replace(value)
}

func genericBrowserEntrypointDocument() assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "application_entrypoint", Path: "src/main.tsx",
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.mount", Static: typeScriptWebMainSource(),
			API: "mount the assembled browser application", DependsOn: []string{"application.render"},
		}},
	}
}

func genericBrowserSmokeTestDocument(
	specification assemblyline.ApplicationSpecification,
) assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "application_smoke_test", Path: "src/App.test.tsx",
		Preamble: `import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import { App } from './App';`,
		Blocks: []assemblyline.SourceBlock{{
			ID: "tests.application_smoke", Static: genericBrowserSmokeTestSource(specification),
			API: "tests assembled application rendering", DependsOn: []string{"application.render"},
		}},
	}
}

func genericBrowserSmokeTestSource(
	specification assemblyline.ApplicationSpecification,
) string {
	return fmt.Sprintf(`describe('assembled application', () => {
  it('renders every accepted capability surface', () => {
    render(<App />);
    expect(screen.getAllByRole('region')).toHaveLength(%d);
  });
});`, len(specification.Requirements))
}

func genericBrowserStylesSource() string {
	return `:root {
	font-family: system-ui, sans-serif;
}

* { box-sizing: border-box; }
body { margin: 0; }
button, input, select, textarea { font: inherit; }
`
}
