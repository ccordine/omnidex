package webresearch

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const exactPortableSemanticLeafCalls = 1

// SemanticCallReceipt is the exact provider-call provenance for one bounded
// semantic station or a code-owned aggregate of bounded semantic leaves.
// Reused is durable accepted-result provenance, never a synonym for zero.
type SemanticCallReceipt struct {
	Calls  int
	Reused bool
}

type semanticCallLedgerEntry struct {
	label        string
	receipt      SemanticCallReceipt
	maximumCalls int
}

// SemanticCallLedger retains each exact typed receipt so callers can
// revalidate provenance instead of trusting a reconstructed counter.
type SemanticCallLedger struct {
	entries []semanticCallLedgerEntry
}

func ValidateSemanticCallReceipt(
	label string,
	receipt SemanticCallReceipt,
	maximumCalls int,
) error {
	if strings.TrimSpace(label) == "" || label != strings.TrimSpace(label) ||
		len(label) > 256 || !utf8.ValidString(label) || strings.ContainsRune(label, '\x00') {
		return fmt.Errorf("web semantic receipt requires one bounded exact label")
	}
	if maximumCalls < exactPortableSemanticLeafCalls {
		return fmt.Errorf("%s has invalid maximum call budget %d", label, maximumCalls)
	}
	if receipt.Reused {
		if receipt.Calls != 0 {
			return fmt.Errorf("%s reuse reported %d provider calls", label, receipt.Calls)
		}
		return nil
	}
	if receipt.Calls < exactPortableSemanticLeafCalls || receipt.Calls > maximumCalls {
		return fmt.Errorf(
			"%s reported %d calls outside the 1..%d bounded leaf budget",
			label, receipt.Calls, maximumCalls,
		)
	}
	return nil
}

func (ledger *SemanticCallLedger) Record(
	label string,
	receipt SemanticCallReceipt,
	maximumCalls int,
) error {
	if ledger == nil {
		return fmt.Errorf("web semantic receipt %q has no ledger", label)
	}
	if err := ValidateSemanticCallReceipt(label, receipt, maximumCalls); err != nil {
		return err
	}
	ledger.entries = append(ledger.entries, semanticCallLedgerEntry{
		label: label, receipt: receipt, maximumCalls: maximumCalls,
	})
	return nil
}

func (ledger *SemanticCallLedger) Merge(
	prefix string,
	other SemanticCallLedger,
) error {
	if ledger == nil {
		return fmt.Errorf("web semantic receipt merge has no destination ledger")
	}
	if strings.TrimSpace(prefix) == "" || prefix != strings.TrimSpace(prefix) ||
		len(prefix) > 128 || !utf8.ValidString(prefix) || strings.ContainsRune(prefix, '\x00') {
		return fmt.Errorf("web semantic receipt merge requires one bounded exact prefix")
	}
	if _, err := other.TotalForSuccess(); err != nil {
		return fmt.Errorf("merge web semantic %s receipts: %w", prefix, err)
	}
	for _, entry := range other.entries {
		label := prefix + ": " + entry.label
		if err := ledger.Record(label, entry.receipt, entry.maximumCalls); err != nil {
			return err
		}
	}
	return nil
}

func (ledger SemanticCallLedger) TotalForSuccess() (SemanticCallReceipt, error) {
	if len(ledger.entries) == 0 {
		return SemanticCallReceipt{}, fmt.Errorf("web semantic work has no exact receipt")
	}
	total := SemanticCallReceipt{Reused: true}
	for _, entry := range ledger.entries {
		if err := ValidateSemanticCallReceipt(
			entry.label, entry.receipt, entry.maximumCalls,
		); err != nil {
			return SemanticCallReceipt{}, err
		}
		total.Calls += entry.receipt.Calls
		total.Reused = total.Reused && entry.receipt.Reused
	}
	if total.Reused {
		if total.Calls != 0 {
			return SemanticCallReceipt{}, fmt.Errorf(
				"web semantic aggregate reuse reported %d provider calls", total.Calls,
			)
		}
		return total, nil
	}
	if total.Calls < exactPortableSemanticLeafCalls {
		return SemanticCallReceipt{}, fmt.Errorf(
			"web semantic aggregate has no fresh calls or durable reuse proof",
		)
	}
	return total, nil
}

func (ledger SemanticCallLedger) ValidateForMaximum(
	label string,
	maximumCalls int,
) (SemanticCallReceipt, error) {
	if maximumCalls < exactPortableSemanticLeafCalls {
		return SemanticCallReceipt{}, fmt.Errorf(
			"%s has invalid aggregate maximum %d", label, maximumCalls,
		)
	}
	receipt, err := ledger.TotalForSuccess()
	if err != nil {
		return SemanticCallReceipt{}, fmt.Errorf("%s: %w", label, err)
	}
	maximumProvenance := 0
	for _, entry := range ledger.entries {
		maximumProvenance += entry.maximumCalls
		if maximumProvenance > maximumCalls {
			return SemanticCallReceipt{}, fmt.Errorf(
				"%s receipt provenance exceeds its %d-call bound",
				label, maximumCalls,
			)
		}
	}
	if receipt.Calls > maximumCalls {
		return SemanticCallReceipt{}, fmt.Errorf(
			"%s reported %d calls above its %d-call bound",
			label, receipt.Calls, maximumCalls,
		)
	}
	return receipt, nil
}

func (ledger SemanticCallLedger) Clone() SemanticCallLedger {
	return SemanticCallLedger{
		entries: append([]semanticCallLedgerEntry(nil), ledger.entries...),
	}
}

func (ledger SemanticCallLedger) Count() int {
	return len(ledger.entries)
}
