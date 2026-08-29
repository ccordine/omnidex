package worker

import "fmt"

type directCodingParserProbe struct {
	AdapterID string
	Path      string
	Source    string
}

type directCodingParserQualification struct {
	ID             string
	StackID        string
	SourceDialects []string
	Probes         []directCodingParserProbe
}

func registeredDirectCodingParserQualifications() []directCodingParserQualification {
	return []directCodingParserQualification{
		{
			ID:      "tree-sitter-typescript-0.23.2-typescript-5.9-profile-v1",
			StackID: genericTypeScriptBrowserAdapter,
			SourceDialects: []string{
				"TypeScript 5.9.3 with TSX react-jsx targeting ECMAScript 2022",
			},
			Probes: []directCodingParserProbe{
				{AdapterID: "typescript", Path: "qualified.ts", Source: "export function qualified(): number { return 1; }\n"},
				{AdapterID: "typescript_react", Path: "Qualified.tsx", Source: "export function Qualified() { return <div />; }\n"},
			},
		},
		{
			ID: "go-parser-go1.24-profile-v1", StackID: genericGoCommandLineAdapter,
			SourceDialects: []string{"Go 1.24.0"},
			Probes: []directCodingParserProbe{{
				AdapterID: "go", Path: "qualified.go", Source: "package main\n\nfunc qualified() int { return 1 }\n",
			}},
		},
		{
			ID:             "tree-sitter-javascript-0.23.1-es2022-profile-v1",
			StackID:        genericJavaScriptCommandLineAdapter,
			SourceDialects: []string{"ECMAScript 2022 modules on Node.js >=22.0.0"},
			Probes: []directCodingParserProbe{{
				AdapterID: "javascript", Path: "qualified.mjs", Source: "export function qualified() { return 1; }\n",
			}},
		},
		{
			ID:             "tree-sitter-rust-0.23.2-edition-2024-profile-v1",
			StackID:        genericRustCommandLineAdapter,
			SourceDialects: []string{"Rust 2024 edition with rust-version 1.85"},
			Probes: []directCodingParserProbe{{
				AdapterID: "rust", Path: "qualified.rs", Source: "pub fn qualified() -> i32 { 1 }\n",
			}},
		},
		{
			ID:             "tree-sitter-java-0.23.5-release-21-profile-v1",
			StackID:        genericJavaCommandLineAdapter,
			SourceDialects: []string{"Java 21 source and class-file API release"},
			Probes: []directCodingParserProbe{{
				AdapterID: "java", Path: "Qualified.java", Source: "final class Qualified { static int value() { return 1; } }\n",
			}},
		},
		{
			ID: "tree-sitter-php-0.23.11-php-8-profile-v1", StackID: genericPHPServiceAdapter,
			SourceDialects: []string{"PHP >=8.2,<9 function syntax"},
			Probes: []directCodingParserProbe{{
				AdapterID: "php", Path: "qualified.php", Source: "<?php\nfunction qualified(): int { return 1; }\n",
			}},
		},
		{
			ID: "tree-sitter-php-0.23.11-laravel-13-profile-v1", StackID: laravelHTTPServiceAdapter,
			SourceDialects: []string{"PHP " + laravelPHPVersion + " function syntax"},
			Probes: []directCodingParserProbe{{
				AdapterID: "php", Path: "qualified.php",
				Source: "<?php\nfunction qualified(): int { return 1; }\n",
			}, {
				AdapterID: "php_executable", Path: "artisan",
				Source: "#!/usr/bin/env php\n<?php\nfunction qualified(): int { return 1; }\n",
			}},
		},
	}
}

func validateDirectCodingParserQualifications(
	adapters map[string]directCodingArtifactAdapter,
	profiles []directCodingProjectVersionProfile,
) error {
	return validateDirectCodingParserQualificationsFrom(
		adapters,
		profiles,
		registeredDirectCodingParserQualifications(),
	)
}

func validateDirectCodingParserQualificationsFrom(
	adapters map[string]directCodingArtifactAdapter,
	profiles []directCodingProjectVersionProfile,
	registeredQualifications []directCodingParserQualification,
) error {
	qualifications := make(map[string]directCodingParserQualification, len(registeredQualifications))
	for index, qualification := range registeredQualifications {
		if qualification.ID == "" || qualification.StackID == "" ||
			len(qualification.SourceDialects) == 0 || len(qualification.Probes) == 0 {
			return fmt.Errorf("parser qualification %d is incomplete", index)
		}
		if _, duplicate := qualifications[qualification.ID]; duplicate {
			return fmt.Errorf("parser qualification %s is registered more than once", qualification.ID)
		}
		seenDialects := make(map[string]struct{}, len(qualification.SourceDialects))
		for _, dialect := range qualification.SourceDialects {
			if dialect == "" {
				return fmt.Errorf("parser qualification %s has an empty source dialect", qualification.ID)
			}
			if _, duplicate := seenDialects[dialect]; duplicate {
				return fmt.Errorf("parser qualification %s repeats source dialect %s", qualification.ID, dialect)
			}
			seenDialects[dialect] = struct{}{}
		}
		for _, probe := range qualification.Probes {
			adapter, exists := adapters[probe.AdapterID]
			if !exists || adapter.Validation.Kind != directCodingArtifactParse {
				return fmt.Errorf("parser qualification %s references non-parser adapter %s", qualification.ID, probe.AdapterID)
			}
			if err := adapter.Validation.Execute(probe.Path, []byte(probe.Source)); err != nil {
				return fmt.Errorf("execute parser qualification %s probe %s: %w", qualification.ID, probe.AdapterID, err)
			}
		}
		qualifications[qualification.ID] = qualification
	}
	for _, profile := range profiles {
		qualification, exists := qualifications[profile.ParserQualification]
		if !exists || qualification.StackID != profile.StackID {
			return fmt.Errorf("project version profile %s references unknown parser qualification %s", profile.ID, profile.ParserQualification)
		}
		qualifiedDialect := false
		for _, dialect := range qualification.SourceDialects {
			if dialect == profile.SourceDialect {
				qualifiedDialect = true
			}
		}
		if !qualifiedDialect {
			return fmt.Errorf(
				"project version profile %s dialect is not proven by parser qualification %s",
				profile.ID, qualification.ID,
			)
		}
		for _, probe := range qualification.Probes {
			if !profileVersionsArtifact(profile, probe.AdapterID) {
				return fmt.Errorf(
					"parser qualification %s probes non-stack adapter %s for profile %s",
					qualification.ID, probe.AdapterID, profile.ID,
				)
			}
		}
	}
	return nil
}

func profileVersionsArtifact(profile directCodingProjectVersionProfile, adapterID string) bool {
	for _, version := range profile.ArtifactVersions {
		if version.AdapterID == adapterID {
			return true
		}
	}
	return false
}
