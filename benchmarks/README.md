# App-build comparison inputs

This directory must never colocate ordinary build requests with evaluation rubrics.

The authoritative comparison boundary is `internal/autonomybench.RunComparison`. Request cases and withheld evaluation plans require separate storage and separate loaders. No app-build comparison CLI is currently registered because the repository does not yet contain a production builder adapter and black-box workspace evaluator.

Do not add a manifest containing both `prompt` and `success_criteria`. That would make the evaluation available before both builds stop and invalidate the comparison.
