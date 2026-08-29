package worker

import (
	"strings"
	"testing"
)

func TestBrowserPublicInteractionSurfaceAllowsRegisteredNonConcealingTailwind(t *testing.T) {
	source := `function View() {
  return <main className="grid grid-cols-2 gap-4 p-6 mx-auto w-full max-w-lg text-center text-lg font-semibold leading-normal border border-solid rounded-xl shadow-lg">
    <button className="px-4 py-2 rounded-md shadow-sm">Apply change</button>
    <p className="col-span-2 pt-4">{result}</p>
  </main>;
}`
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(source); err != nil {
		t.Fatalf("registered non-concealing Tailwind utilities were rejected: %v", err)
	}
}

func TestBrowserPublicInteractionSurfaceRejectsNonAllowlistedConcealmentClasses(t *testing.T) {
	fixtures := map[string]string{
		"arbitrary decimal opacity on booking control": `function View() { return <button className="opacity-[0.0]">Reserve room</button>; }`,
		"arbitrary zero scale on billing control":      `function View() { return <button className="scale-[0]">Pay invoice</button>; }`,
		"arbitrary zero width on inventory control":    `function View() { return <input aria-label="Stock code" className="w-[0px]" />; }`,
		"arbitrary zero height on travel control":      `function View() { return <textarea aria-label="Trip notes" className="h-[0px]" />; }`,
		"transparent foreground on scheduling control": `function View() { return <button className="text-transparent">Publish schedule</button>; }`,
		"off-screen transform on reporting control":    `function View() { return <button className="-translate-x-full">Export report</button>; }`,
		"off-screen transform on dynamic result owner": `function View() { return <main><button>Recalculate forecast</button><p className="translate-x-full">{forecast}</p></main>; }`,
		"arbitrary opacity on dynamic result owner":    `function View() { return <main><button>Update estimate</button><p className="opacity-[0.0]">{estimate}</p></main>; }`,
		"transparent dynamic result owner":             `function View() { return <main><button>Refresh capacity</button><p className="text-transparent">{capacity}</p></main>; }`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "non-allowlisted Tailwind class") {
				t.Fatalf("concealing class was not rejected: %v", err)
			}
		})
	}
}

func TestBrowserPublicInteractionSurfaceRejectsUnknownTailwindClass(t *testing.T) {
	_, err := extractDirectCodingBrowserPublicInteractionSurface(
		`function View() { return <button className="custom-application-control">Continue</button>; }`,
	)
	if err == nil || !strings.Contains(err.Error(), "non-allowlisted Tailwind class") {
		t.Fatalf("unknown class was not rejected: %v", err)
	}
}

func TestBrowserPublicInteractionSurfaceTailwindAllowlistRejectsUnsafeCategories(t *testing.T) {
	for _, class := range []string{
		"opacity-100", "bg-white", "text-black", "absolute", "left-0", "z-50",
		"transform-gpu", "scale-100", "overflow-auto", "pointer-events-none",
		"animate-pulse", "p-[1rem]", "inventory", "contents", "mt-4", "ml-auto",
	} {
		t.Run(class, func(t *testing.T) {
			if err := validateDirectCodingBrowserSafeTailwindClass(class); err == nil {
				t.Fatalf("unsafe or unknown class %q was accepted", class)
			}
		})
	}
	for _, class := range []string{
		"sm:grid-cols-2", "focus-visible:border-2", "rounded-t", "tracking-tight",
	} {
		t.Run(class, func(t *testing.T) {
			if err := validateDirectCodingBrowserSafeTailwindClass(class); err != nil {
				t.Fatalf("registered safe class %q was rejected: %v", class, err)
			}
		})
	}
}
