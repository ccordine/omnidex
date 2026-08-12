package cognitionreplay

import (
	"bytes"
	"testing"
)

func TestPrivateOverlayIsSeparateDeterministicAndBaseBound(t *testing.T) {
	base, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	input := validPrivateOverlayInput(t, base)
	first, err := ExportPrivateOverlay(input, base.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExportPrivateOverlay(input, base.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes, second.Bytes) || first.SHA256 != second.SHA256 {
		t.Fatal("identical private inputs did not produce a byte-identical overlay")
	}
	if _, err := VerifyPrivateOverlay(first.Bytes, base.Bytes); err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyPrivateOverlay(first.Bytes, base.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Manifest().Schema != PrivateOverlaySchemaV2 ||
		verified.Manifest().TerminalAuthoritySHA256 == "" {
		t.Fatalf("private v2 terminal binding=%+v", verified.Manifest())
	}
	if _, err := VerifyBase(first.Bytes); err == nil {
		t.Fatal("private overlay was accepted as a public base replay")
	}
	other := append([]byte(nil), base.Bytes...)
	other[len(other)-1] ^= 1
	if _, err := VerifyPrivateOverlay(first.Bytes, other); err == nil {
		t.Fatal("private overlay accepted a different base artifact")
	}
}

func TestPrivateOverlayTerminalAuthorityMustBeASealedEpisode(t *testing.T) {
	sealed := validBaseInput(t).TerminalAuthority
	if err := requirePrivateOverlayTerminal(sealed); err != nil {
		t.Fatal(err)
	}
	if err := requirePrivateOverlayTerminal(validPreEpisodeTerminalForTest(t)); err == nil {
		t.Fatal("private overlay accepted a pre-episode provider failure")
	}
}

func TestPrivateOverlayRejectsOrphanTruthData(t *testing.T) {
	base, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	input := validPrivateOverlayInput(t, base)
	input.Blobs = append(input.Blobs, testBlob(t, `{"orphan":true}`))
	if _, err := ExportPrivateOverlay(input, base.Bytes); err == nil {
		t.Fatal("private overlay accepted an orphan blob")
	}
}

func TestPrivateOverlayRejectsInexactPrivateAuthority(t *testing.T) {
	base, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*PrivateOverlayInput){
		"wrong oracle binding": func(input *PrivateOverlayInput) {
			input.OracleSHA256 = testDigest("different-oracle")
		},
		"missing oracle source": func(input *PrivateOverlayInput) {
			input.Sources = input.Sources[1:]
		},
		"missing evaluation source": func(input *PrivateOverlayInput) {
			input.Sources = input.Sources[:1]
		},
		"reordered sources": func(input *PrivateOverlayInput) {
			input.Sources[0], input.Sources[1] = input.Sources[1], input.Sources[0]
		},
		"evaluation collapsed into world truth": func(input *PrivateOverlayInput) {
			input.Events[1].Kind = PrivateEventWorldTruth
		},
		"world truth cites evaluation": func(input *PrivateOverlayInput) {
			input.Events[0].Sources = []PrivateSourceRef{input.Sources[1].Ref()}
		},
		"evaluation omits evaluation source": func(input *PrivateOverlayInput) {
			input.Events[1].Sources = []PrivateSourceRef{input.Sources[0].Ref()}
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			input := validPrivateOverlayInput(t, base)
			mutate(&input)
			if _, err := ExportPrivateOverlay(input, base.Bytes); err == nil {
				t.Fatal("private overlay accepted inexact private authority")
			}
		})
	}
}

func TestPrivateOverlayVerifierRejectsAlteredMissingReorderedAndOrphanData(t *testing.T) {
	base, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := ExportPrivateOverlay(validPrivateOverlayInput(t, base), base.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	entries := readTestContainer(t, artifact.Bytes)
	for name, mutate := range map[string]func([]testContainerEntry) []testContainerEntry{
		"altered blob": func(values []testContainerEntry) []testContainerEntry {
			values[len(values)-1].Body = append(values[len(values)-1].Body, '!')
			return values
		},
		"missing source page": func(values []testContainerEntry) []testContainerEntry {
			return append(values[:1], values[2:]...)
		},
		"reordered pages": func(values []testContainerEntry) []testContainerEntry {
			values[1], values[2] = values[2], values[1]
			return values
		},
		"orphan entry": func(values []testContainerEntry) []testContainerEntry {
			return append(values, testContainerEntry{
				Name: "blobs/sha256/" + testDigest("orphan"), Body: []byte("orphan"),
			})
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			candidate := mutate(cloneTestEntries(entries))
			if _, err := VerifyPrivateOverlay(writeTestContainer(t, candidate), base.Bytes); err == nil {
				t.Fatal("corrupt private replay overlay was accepted")
			}
		})
	}
}
