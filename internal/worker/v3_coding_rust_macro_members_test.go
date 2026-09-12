package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRustMacroAssociatedItemsAreNotFreeValues(t *testing.T) {
	input := assemblyline.FragmentGenerationInput{Language: "rust", Dialect: "Rust 2024", Signature: "fn observe() -> String", Behavior: "Return the observed value as text."}
	for _, body := range []string{
		`format!("{}", String::from("value"))`,
		`format!("{}", Option::Some(2).unwrap())`,
		`format!("{}", String /* type */ :: /* member */ from("value"))`,
	} {
		if _, err := validateDirectCodingRustFragment(input, body); err != nil {
			t.Errorf("rejected known associated item in %s: %v", body, err)
		}
	}
	for _, body := range []string{
		`format!("{}", Missing::from("value"))`,
		`format!("{}", String::std::from("value"))`,
		`format!("{}", std::fs::read_to_string("value"))`,
	} {
		if _, err := validateDirectCodingRustFragment(input, body); err == nil {
			t.Errorf("accepted unavailable root or component in %s", body)
		}
	}
}
