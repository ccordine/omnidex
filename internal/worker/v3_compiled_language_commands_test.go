package worker

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCompiledLanguageCommandsUseExactRegisteredTools(t *testing.T) {
	parent := t.TempDir()
	root, output := filepath.Join(parent, "source"), filepath.Join(parent, "output")
	for _, stackID := range []string{genericJavaScriptCommandLineAdapter, genericRustCommandLineAdapter, genericJavaCommandLineAdapter} {
		t.Run(stackID, func(t *testing.T) {
			program := testCompiledLanguageVerificationProgram(t, stackID)
			assembly, err := directCodingAssemblyFromProgram(program)
			if err != nil {
				t.Fatal(err)
			}
			for _, complete := range []bool{false, true} {
				commands, err := directCodingCompiledLanguageCommands(program.Project.Profile, assembly, root, output, complete)
				if err != nil {
					t.Fatal(err)
				}
				for _, command := range commands {
					if command.Timeout != defaultDirectCodingVerificationTimeout || !sort.StringsAreSorted(command.Environment) {
						t.Fatalf("unbounded or unordered command: %+v", command)
					}
				}
				var want [][]string
				switch stackID {
				case genericJavaScriptCommandLineAdapter:
					want = [][]string{
						{"node", "--check", "feature001.mjs"},
						{"node", "--check", "feature001.test.mjs"},
						{"node", "--check", "main.mjs"},
						{"node", "--check", "runtime.mjs"},
						{"node", "--input-type=module", "--eval", "await import(\"./feature001.mjs\");\nawait import(\"./main.mjs\");\nawait import(\"./runtime.mjs\");\n"},
						{"node", "--test", "--test-concurrency=1", "feature001.test.mjs"},
					}
				case genericRustCommandLineAdapter:
					want = [][]string{
						{"cargo", "check", "--locked", "--offline", "--lib"},
						{"cargo", "test", "--locked", "--offline", "--lib", "--", "--test-threads=1"},
					}
					if complete {
						want = append(want, []string{"cargo", "build", "--locked", "--offline"})
					}
					for _, command := range commands {
						wantEnvironment := []string{"CARGO_HOME=" + filepath.Join(output, "cargo"), "CARGO_INCREMENTAL=0", "CARGO_TARGET_DIR=" + filepath.Join(output, "target")}
						if !reflect.DeepEqual(command.Environment, wantEnvironment) {
							t.Fatalf("Cargo cache escaped owned output: %v", command.Environment)
						}
					}
				case genericJavaCommandLineAdapter:
					classes := filepath.Join(output, "classes")
					want = [][]string{
						{"javac", "--release", "21", "-encoding", "UTF-8", "-Xlint:all", "-Werror", "-d", classes, "Feature001.java", "Feature001Test.java", "Main.java", "Runtime.java"},
						{"java", "-ea", "-cp", classes, "Feature001Test"},
					}
					if complete {
						want = append(want, []string{"jar", "--create", "--file", filepath.Join(output, "application.jar"), "--main-class", "Main", "-C", classes, "."})
					}
				}
				got := make([][]string, 0, len(commands))
				for _, command := range commands {
					got = append(got, command.Argv)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("complete=%t commands=%v; want %v", complete, got, want)
				}
			}
		})
	}
}

func TestCompiledLanguageCommandsRejectMissingSourceAndUnknownStack(t *testing.T) {
	for _, stackID := range []string{genericJavaScriptCommandLineAdapter, genericRustCommandLineAdapter, genericJavaCommandLineAdapter, "unknown"} {
		profile := directCodingProjectVersionProfile{StackID: stackID}
		if _, err := directCodingCompiledLanguageCommands(profile, directCodingAssembly{}, "/stage/source", "/stage/output", true); err == nil {
			t.Errorf("stack %s accepted no source", stackID)
		}
	}
}

func TestCompiledLanguageCommandsRejectMissingOwnedBehavioralTests(t *testing.T) {
	for stackID, source := range map[string]string{
		genericJavaScriptCommandLineAdapter: "feature001.mjs",
		genericRustCommandLineAdapter:       "src/lib.rs",
		genericJavaCommandLineAdapter:       "Feature001.java",
	} {
		assembly := directCodingAssembly{Files: []directCodingFileTask{{Path: source}}}
		if _, err := directCodingCompiledLanguageCommands(directCodingProjectVersionProfile{StackID: stackID}, assembly, "/stage/source", "/stage/output", true); err == nil {
			t.Errorf("stack %s accepted compilation without owned tests", stackID)
		}
	}
}

func TestCompilerOutputCleanupCannotOverlapSource(t *testing.T) {
	profile := directCodingProjectVersionProfile{StackID: genericRustCommandLineAdapter}
	assembly := directCodingAssembly{Files: []directCodingFileTask{{Path: "src/lib.rs"}}}
	for _, paths := range [][2]string{
		{"/stage/source", "/stage/source"},
		{"/stage/source", "/stage/source/output"},
		{"/stage/source", "/stage"},
		{"/stage/source", "/"},
		{"relative", "/stage/output"},
		{"/stage/source", "relative"},
		{"/stage/source/../source", "/stage/output"},
		{"/stage/source", "/stage/output/../output"},
	} {
		if _, err := directCodingCompiledLanguageCommands(profile, assembly, paths[0], paths[1], true); err == nil || !strings.Contains(err.Error(), "director") && !strings.Contains(err.Error(), "source") {
			t.Errorf("unsafe source/output pair %v: %v", paths, err)
		}
	}
}

func TestCompilationSourcePathsRejectInvalidOrRepeatedPaths(t *testing.T) {
	for _, paths := range [][]string{{"../feature.mjs"}, {"/feature.mjs"}, {"feature.mjs", "feature.mjs"}, {"unsupported.unknown"}} {
		var assembly directCodingAssembly
		for _, path := range paths {
			assembly.Files = append(assembly.Files, directCodingFileTask{Path: path})
		}
		if _, err := directCodingCompilationSourcePaths(assembly, "javascript"); err == nil {
			t.Errorf("accepted invalid compilation paths: %v", paths)
		}
	}
}
