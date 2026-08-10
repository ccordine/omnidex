package main

import (
	"os"
	"strings"
	"testing"
)

func TestResumeCommandsAreRegisteredWithStrictArguments(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	tests := []struct {
		phase string
		want  string
	}{
		{phase: "prepare-resume", want: "prepare-resume requires exactly --request and --config"},
		{phase: "resume", want: "resume requires exactly --config"},
		{phase: "verify-resume", want: "verify-resume requires exactly --config"},
	}
	for _, test := range tests {
		t.Run(test.phase, func(t *testing.T) {
			os.Args = []string{"cognition-gauntlet", test.phase}
			err := run()
			if err == nil || err.Error() != test.want {
				t.Fatalf("error=%v want=%q", err, test.want)
			}
		})
	}
}

func TestUsagePublishesFreshProcessResumeVerifier(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = []string{"cognition-gauntlet"}
	err := run()
	if err == nil || !strings.Contains(err.Error(), "prepare-resume") ||
		!strings.Contains(err.Error(), "verify-resume") {
		t.Fatalf("usage error=%v", err)
	}
}

func TestTransferCommandsAreRegisteredWithStrictArguments(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	tests := []struct {
		phase string
		want  string
	}{
		{phase: "prepare-transfer", want: "prepare-transfer requires exactly --request and --config"},
		{phase: "transfer", want: "transfer requires exactly --config"},
		{phase: "verify-transfer", want: "verify-transfer requires exactly --config"},
	}
	for _, test := range tests {
		t.Run(test.phase, func(t *testing.T) {
			os.Args = []string{"cognition-gauntlet", test.phase}
			err := run()
			if err == nil || err.Error() != test.want {
				t.Fatalf("error=%v want=%q", err, test.want)
			}
		})
	}
}

func TestScaleCommandsAreRegisteredWithStrictArguments(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	tests := []struct {
		phase string
		want  string
	}{
		{phase: "prepare-scale", want: "prepare-scale requires exactly --request and --config"},
		{phase: "scale", want: "scale requires exactly --config"},
		{phase: "verify-scale", want: "verify-scale requires exactly --config"},
	}
	for _, test := range tests {
		t.Run(test.phase, func(t *testing.T) {
			os.Args = []string{"cognition-gauntlet", test.phase}
			err := run()
			if err == nil || err.Error() != test.want {
				t.Fatalf("error=%v want=%q", err, test.want)
			}
		})
	}
}
