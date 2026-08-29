package llm

import (
	"math"
	"testing"
)

func TestValidateResponseContractAcceptsRawTransportSampling(t *testing.T) {
	zero := ExactPreparedTemperature(0)
	if err := ValidateResponseContract(PreparedModel{Temperature: &zero}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateResponseContractRejectsInvalidSampling(t *testing.T) {
	for name, value := range map[string]ExactPreparedTemperature{
		"negative":      -0.1,
		"above maximum": 2.1,
		"not a number":  ExactPreparedTemperature(math.NaN()),
		"infinity":      ExactPreparedTemperature(math.Inf(1)),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateResponseContract(PreparedModel{Temperature: &value}); err == nil {
				t.Fatal("invalid sampling authority was accepted")
			}
		})
	}
}
