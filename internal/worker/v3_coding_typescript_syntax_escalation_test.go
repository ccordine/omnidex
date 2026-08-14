package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptSyntaxCorrectionEscalatesFromLocalWindowsToWholeDeclaration(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name      string
		block     assemblyline.TypeScriptBlock
		tsx       bool
		invalid   string
		corrected string
	}{
		{
			name: "numeric calculation",
			block: assemblyline.TypeScriptBlock{
				ID: "calculation.adjust", Signature: "function adjust(value: number): number",
				Contract: "Return the adjusted value.", API: "function adjust(value: number): number",
			},
			invalid: strings.Join([]string{
				"function adjust(value: number): number {",
				"  const keepOne = value + 1;",
				"  const keepTwo = keepOne + 1;",
				"  const keepThree = keepTwo + 1;",
				"  const keepFour = keepThree + 1;",
				"  const marker = 1;",
				"  const broken = value + ;",
				"  return keepFour + marker;",
				"}",
			}, "\n"),
			corrected: "function adjust(value: number): number { return value + 1; }",
		},
		{
			name: "visual panel",
			block: assemblyline.TypeScriptBlock{
				ID: "view.panel", Signature: "function Panel(): ReactElement",
				Contract: "Render the current marker.", API: "function Panel(): ReactElement",
			},
			tsx: true,
			invalid: strings.Join([]string{
				"function Panel(): ReactElement {",
				"  const keepOne = 'one';",
				"  const keepTwo = 'two';",
				"  const keepThree = 'three';",
				"  const keepFour = 'four';",
				"  const marker = 1;",
				"  const broken = marker + ;",
				"  return <section>{keepOne + keepTwo + keepThree + keepFour + marker}</section>;",
				"}",
			}, "\n"),
			corrected: "function Panel(): ReactElement { return <section>ready</section>; }",
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			localLineCounts := make([]int, 0, 2)
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, CorrectionModel: "corrector",
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					candidate := fixture.invalid
					if calls > 1 {
						var correction assemblyline.FragmentCorrectionInput
						if err := json.Unmarshal(job.Payload, &correction); err != nil {
							t.Fatal(err)
						}
						if calls <= 3 {
							if correction.RepairRegion == nil || correction.CurrentDeclaration != "" {
								t.Fatalf("call %d did not retain local-only authority: %#v", calls, correction)
							}
							region := correction.RepairRegion
							localLineCounts = append(localLineCounts, region.EndLine-region.StartLine+1)
							replacement := strings.Replace(
								region.Source,
								"marker = "+string(rune('0'+calls-1)),
								"marker = "+string(rune('0'+calls)),
								1,
							)
							if replacement == region.Source {
								t.Fatalf("call %d local region omitted its marker: %#v", calls, region)
							}
							candidate = replacement
						} else {
							if correction.RepairRegion != nil ||
								strings.TrimSpace(correction.CurrentDeclaration) == "" {
								t.Fatalf("whole-declaration escalation authority=%#v", correction)
							}
							candidate = fixture.corrected
						}
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", directCodingTypeScriptFragmentJob{
				block: fixture.block, tsx: fixture.tsx,
			})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 4 || len(localLineCounts) != 2 || localLineCounts[1] <= localLineCounts[0] {
				t.Fatalf("calls=%d local windows=%v", calls, localLineCounts)
			}
			if strings.TrimSpace(source) != fixture.corrected {
				t.Fatalf("source=%q want %q", source, fixture.corrected)
			}
		})
	}
}

func TestTypeScriptSyntaxCorrectionEscalatesImmediatelyWhenLocalRegionCannotFit(t *testing.T) {
	t.Parallel()

	longLiteral := strings.Repeat("x", 1200)
	fixtures := []struct {
		name      string
		block     assemblyline.TypeScriptBlock
		tsx       bool
		invalid   string
		corrected string
	}{
		{
			name: "long TypeScript line beside numeric syntax failure",
			block: assemblyline.TypeScriptBlock{
				ID: "calculation.total", Signature: "function total(value: number): number",
				Contract: "Return one numeric total.", API: "function total(value: number): number",
			},
			invalid: strings.Join([]string{
				"function total(value: number): number {",
				`  const evidence = "` + longLiteral + `";`,
				"  return value + ;",
				"}",
			}, "\n"),
			corrected: "function total(value: number): number { return value + 1; }",
		},
		{
			name: "long TSX line beside element syntax failure",
			block: assemblyline.TypeScriptBlock{
				ID: "view.status", Signature: "function Status(): ReactElement",
				Contract: "Render one status element.", API: "function Status(): ReactElement",
			},
			tsx: true,
			invalid: strings.Join([]string{
				"function Status(): ReactElement {",
				`  const evidence = "` + longLiteral + `";`,
				"  return <output>{evidence}</output",
				"}",
			}, "\n"),
			corrected: `function Status(): ReactElement { return <output>ready</output>; }`,
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, CorrectionModel: "corrector",
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					candidate := fixture.invalid
					if calls == 2 {
						var correction assemblyline.FragmentCorrectionInput
						if err := json.Unmarshal(job.Payload, &correction); err != nil {
							t.Fatal(err)
						}
						if correction.RepairRegion != nil {
							t.Fatalf("unrepresentable local window retained region authority: %#v", correction.RepairRegion)
						}
						if correction.CurrentDeclaration != fixture.invalid {
							t.Fatalf("whole-declaration escalation lost exact current source")
						}
						candidate = fixture.corrected
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", directCodingTypeScriptFragmentJob{
				block: fixture.block, tsx: fixture.tsx,
			})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("syntax correction dispatched %d calls, want immediate whole-declaration correction", calls)
			}
			if source != fixture.corrected {
				t.Fatalf("source=%q want=%q", source, fixture.corrected)
			}
		})
	}
}
