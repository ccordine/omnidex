package taskstate

import (
	"errors"
	"testing"
)

func TestValidateRefUsesCanonicalSchemeSuffixGrammar(t *testing.T) {
	t.Parallel()
	ref := Ref{
		URI: "repo:snapshot/abc/symbol/one", Version: "v1",
		Hash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Relation: RefSource,
	}
	if err := ValidateRef(ref); err != nil {
		t.Fatalf("validate scheme:suffix reference: %v", err)
	}
	ref.URI = "repo:"
	if err := ValidateRef(ref); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("empty suffix error = %v, want ErrInvalidCommand", err)
	}
	ref.URI = "repo:snapshot/abc\u2003symbol/one"
	if err := ValidateRef(ref); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("Unicode whitespace error = %v, want ErrInvalidCommand", err)
	}
	ref.URI = "repo:snapshot/abc\x00symbol/one"
	if err := ValidateRef(ref); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("NUL URI error = %v, want ErrInvalidCommand", err)
	}
}

func TestExactTextRejectsPostgreSQLForbiddenNUL(t *testing.T) {
	t.Parallel()
	if err := requireExactText("valid\x00invalid", "test text"); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("NUL exact text error = %v, want ErrInvalidCommand", err)
	}
	invalidUTF8 := string([]byte{'v', 'a', 'l', 'i', 'd', 0xff})
	if err := requireExactText(invalidUTF8, "test text"); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("invalid UTF-8 exact text error = %v, want ErrInvalidCommand", err)
	}
	ref := Ref{
		URI: invalidUTF8, Version: "v1",
		Hash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Relation: RefSource,
	}
	if err := ValidateRef(ref); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("invalid UTF-8 URI error = %v, want ErrInvalidCommand", err)
	}
}

func TestRefIdentityExcludesHashAndValidateRefsRejectsHashConflict(t *testing.T) {
	t.Parallel()
	first := Ref{
		URI: "repo:snapshot/abc/symbol/one", Version: "v1",
		Hash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Relation: RefSource,
	}
	second := first
	second.Hash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if RefIdentity(first) != RefIdentity(second) {
		t.Fatal("content hash incorrectly became part of stable reference identity")
	}
	if err := validateRefs([]Ref{first, second}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("hash-conflicting identity error = %v, want ErrInvalidState", err)
	}
}
