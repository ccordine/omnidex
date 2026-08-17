package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericBrowserAppDocument(
	specification assemblyline.ApplicationSpecification,
	targetTree assemblyline.TargetTree,
) (assemblyline.TypeScriptDocument, error) {
	imports := []string{
		"import { useMemo } from 'react';",
		"import type { ReactElement } from 'react';",
		"import { createApplicationRuntime, createFeatureRuntime } from './runtime';",
	}
	dependencies := []string{"runtime.factory"}
	for index := range specification.Requirements {
		files, err := targetTree.RequirementFiles(specification.Requirements[index].ID)
		if err != nil {
			return assemblyline.TypeScriptDocument{}, err
		}
		name := fmt.Sprintf("Feature%03d", index+1)
		imports = append(imports, fmt.Sprintf("import { %s } from '%s';", name, typeScriptRelativeModule("src/App.tsx", files.ImplementationPath)))
		dependencies = append(dependencies, fmt.Sprintf("feature.%03d", index+1))
	}
	return assemblyline.TypeScriptDocument{
		ID: "application_shell", Path: "src/App.tsx", Header: strings.Join(imports, "\n"),
		Blocks: []assemblyline.TypeScriptBlock{{
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
			sequence, strconv.Quote(genericBrowserCapabilityID(sequence)),
		))
	}
	body.WriteString("  }), [runtime]);\n")
	body.WriteString("  return (\n")
	body.WriteString("    <main className=\"application-shell\">\n")
	body.WriteString("      <header className=\"application-header\">\n")
	body.WriteString("        <p className=\"application-kicker\">Live workspace</p>\n")
	body.WriteString("        <h1>" + escapeTypeScriptJSXText(specification.ProductQuote) + "</h1>\n")
	body.WriteString("      </header>\n")
	body.WriteString("      <div className=\"capability-grid\">\n")
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		body.WriteString(fmt.Sprintf(
			"        <section className=\"capability-slot\" aria-label={%s}>\n",
			strconv.Quote(requirement.SourceQuote),
		))
		body.WriteString(fmt.Sprintf("          <Feature%03d runtime={features.feature%03d} />\n", sequence, sequence))
		body.WriteString("        </section>\n")
	}
	body.WriteString("      </div>\n")
	body.WriteString("    </main>\n")
	body.WriteString("  );\n")
	body.WriteString("}")
	return body.String()
}

func escapeTypeScriptJSXText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "{", "&#123;", "}", "&#125;").Replace(value)
}

func genericBrowserSmokeTestDocument(
	specification assemblyline.ApplicationSpecification,
) assemblyline.TypeScriptDocument {
	return assemblyline.TypeScriptDocument{
		ID: "application_smoke_test", Path: "src/App.test.tsx",
		Header: `import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import { App } from './App';`,
		Blocks: []assemblyline.TypeScriptBlock{{
			ID: "tests.application_smoke", Static: genericBrowserSmokeTestSource(specification),
			API: "tests assembled application rendering", DependsOn: []string{"application.render"},
		}},
	}
}

func genericBrowserSmokeTestSource(specification assemblyline.ApplicationSpecification) string {
	return fmt.Sprintf(`describe('assembled application', () => {
  it('renders the accepted product and every capability without browser resources', () => {
    const view = render(<App />);
    expect(screen.getByRole('main')).not.toBeNull();
    expect(screen.getByRole('heading', { name: %s })).not.toBeNull();
    expect(view.container.querySelectorAll('.capability-slot')).toHaveLength(%d);
  });
});`, strconv.Quote(specification.ProductQuote), len(specification.Requirements))
}

func genericBrowserStylesSource() string {
	return `:root {
  color: #e8edf4;
  background: #0b0e14;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-synthesis: none;
  text-rendering: optimizeLegibility;
}

* { box-sizing: border-box; }
body { min-width: 320px; min-height: 100vh; margin: 0; }
button, input, select, textarea { font: inherit; }
button, input, select, textarea { accent-color: #7dd3fc; }
button { cursor: pointer; }
button:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible {
  outline: 3px solid #7dd3fc;
  outline-offset: 2px;
}
.application-shell { width: min(1200px, 100%); min-height: 100vh; margin: 0 auto; padding: 24px; }
.application-header { margin-bottom: 20px; }
.application-header h1 { margin: 4px 0 0; font-size: clamp(2rem, 6vw, 4rem); }
.application-kicker { margin: 0; color: #7dd3fc; font-size: .78rem; font-weight: 800; letter-spacing: .12em; text-transform: uppercase; }
.capability-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr)); gap: 16px; }
.capability-slot { min-width: 0; padding: 18px; border: 1px solid #263244; border-radius: 16px; background: #121824; box-shadow: 0 16px 40px rgb(0 0 0 / .18); }
.capability-slot section { min-width: 0; }
.capability-slot button, .capability-slot input, .capability-slot select, .capability-slot textarea {
  max-width: 100%; min-height: 42px; border: 1px solid #334155; border-radius: 10px; color: inherit; background: #182131;
}
.capability-slot button { padding: 9px 14px; }
.capability-slot input, .capability-slot select, .capability-slot textarea { padding: 8px 10px; }
@media (max-width: 640px) { .application-shell { padding: 14px; } }
`
}
