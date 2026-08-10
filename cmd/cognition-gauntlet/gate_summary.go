package main

import "fmt"

func gateEvidenceSummary(
	artifact string,
	count int,
	qualified bool,
	promotionEligible bool,
) string {
	return fmt.Sprintf(
		"gate evidence qualified %t; product promotion eligible %t; %s %d\n",
		qualified, promotionEligible, artifact, count,
	)
}
