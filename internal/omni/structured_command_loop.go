package omni

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runStructuredCommandLoop(ctx context.Context, prompt string, referenceHistory []Message, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, cfg structuredCommandDecisionRunConfig, evaluator StructuredLLMResponseEvaluator, evaluatorThreshold int, ledger []StructuredObjective, minimalContext MinimalContext, selectedRecipes []Recipe, worksiteSurvey WorksiteSurvey, result CommandDecisionResult) (CommandDecisionResult, error) {
	lastCompletionCheckedObservationCount := 0
	for step := 1; step <= defaultCommandDecisionMaxSteps; step++ {
		disposition, err := prepareStructuredPlannerStep(
			ctx,
			step,
			prompt,
			&cfg,
			&worksiteSurvey,
			&ledger,
			selectedRecipes,
			evaluator,
			evaluatorThreshold,
			&lastCompletionCheckedObservationCount,
			onEvent,
			&result,
		)
		if err != nil {
			return result, err
		}
		if disposition == structuredStepComplete {
			return result, nil
		}
		if disposition == structuredStepRetry {
			continue
		}
		gateDecision := ProgressionGate{MaxRecoveryAttempts: 4}.ReviewStep(ProgressionInput{
			Prompt:          prompt,
			WorkingDir:      cfg.CurrentWorkingDirectory,
			WorksiteSurvey:  worksiteSurvey,
			ObjectiveLedger: ledger,
			Observations:    result.Observations,
		})
		if gateDecision.Action == ProgressFailWithEvidence {
			emitStructuredCommandEvent(onEvent, "progression_gate_failed", "Progression gate exhausted recovery routes", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": gateDecision.Reason,
			})
			result.PartialProgress = hasSuccessfulCommandObservation(result.Observations) || len(result.Observations) > 0
			if result.ExitCode == 0 {
				result.ExitCode = 1
			}
			return result, CommandDecisionExhaustedError{MaxSteps: step}
		}
		if (gateDecision.Action == ProgressForceRecovery || gateDecision.Action == ProgressUseCompletedEvidence) && cfg.ShellSpecialist != nil && worksiteSurvey.TaskMode != TaskModeResearchOnly {
			handled, err := runProgressionGateRecovery(ctx, step, prompt, gateDecision, cfg, worksiteSurvey, stdout, stderr, onEvent, onAsk, &result)
			if err != nil {
				return result, err
			}
			if handled {
				continue
			}
		}
		if handled, err := routeActiveToolchainRepairChildBeforePlanner(ctx, step, prompt, cfg, worksiteSurvey, stdout, stderr, onEvent, onAsk, &result); handled || err != nil {
			if err != nil {
				return result, err
			}
			continue
		}
		basePlannerReq := buildBudgetedStructuredPlannerRequest(
			step,
			prompt,
			referenceHistory,
			cfg,
			result.Observations,
			ledger,
			minimalContext,
			worksiteSurvey,
			onEvent,
		)
		emitStructuredCommandEvent(onEvent, "structured_llm_request_started", "Requesting next structured command decision", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"pending_objectives": pendingStructuredObjectiveIDs(ledger),
			"completed_actions":  fmt.Sprintf("%d", len(completedActionsFromState(ledger, result.Observations))),
			"loop_state":         structuredLoopStateFromState(ledger, result.Observations).Status,
		})
		if client == nil {
			if result.ExitCode == 0 {
				result.ExitCode = 1
			}
			return result, fmt.Errorf("llm client is required for planner step")
		}
		resp, err := requestStructuredCommandPayload(ctx, client, basePlannerReq, step, onEvent)
		if err != nil {
			if hasSuccessfulCommandObservation(result.Observations) {
				result.PartialProgress = true
				emitStructuredCommandEvent(onEvent, "structured_planner_failed_after_progress", "Planner request failed after successful command progress", map[string]string{
					"step":               fmt.Sprintf("%d", step),
					"error":              truncateStructuredTimelineValue(err.Error()),
					"pending_objectives": pendingStructuredObjectiveIDs(ledger),
				})
			} else if result.ExitCode == 0 {
				result.ExitCode = 1
			}
			return result, err
		}

		payload, err := ParseStructuredCommandPayload(resp.Content)
		if err != nil {
			return result, err
		}
		payload.Command = normalizeStructuredCommand(payload.Command)
		if evaluator != nil && shouldRunBroadEvaluatorForPlannerPayload(payload) && !hasImplementationArchitectProgress(result.Observations) {
			if len(pendingStructuredObjectives(ledger)) > 0 {
				emitStructuredCommandEvent(onEvent, "structured_evaluator_deferred_for_objective_queue", "Broad evaluator deferred until top-level objective queue completes", map[string]string{
					"step":               fmt.Sprintf("%d", step),
					"pending_objectives": pendingStructuredObjectiveIDs(ledger),
				})
			} else if requiresTypedWorkQueueCompletion(&result) && !typedWorkQueuePassedForCompletion(&result) {
				emitStructuredCommandEvent(onEvent, "structured_evaluator_deferred_for_typed_queue", "Broad evaluator deferred until typed work queue passes", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
			} else if shouldDeferBroadEvaluatorForArchitectCompletion(payload, prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_evaluator_deferred_for_architect_queue", "Broad evaluator deferred until architect repair queue completes", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "architect_current_item_pending",
				})
			} else {
				accepted, repairedResp, disabledEvaluator, err := evaluateAndRepairPlannerResponse(ctx, step, prompt, client, basePlannerReq, resp, evaluator, evaluatorThreshold, cfg, worksiteSurvey, ledger, result.Observations, onEvent, &result)
				if err != nil {
					return result, err
				}
				resp = repairedResp
				if disabledEvaluator {
					evaluator = nil
				}
				if !accepted {
					continue
				}
				payload, err = ParseStructuredCommandPayload(resp.Content)
				if err != nil {
					return result, err
				}
				payload.Command = normalizeStructuredCommand(payload.Command)
			}
		} else if evaluator != nil && shouldBypassEvaluatorForArchitectImplementation(resp.Content, prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, result.Observations) {
			emitStructuredCommandEvent(onEvent, "structured_evaluator_bypassed_for_architect", "Evaluator bypassed for architect-controlled implementation step", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "architect contract and deterministic validators own current implementation work item",
			})
		} else if evaluator != nil {
			emitStructuredCommandEvent(onEvent, "structured_evaluator_deferred_for_scoped_validation", "Broad evaluator deferred until completion; scoped validators own this step", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "non-final payload",
			})
		}
		ledger = mergePlannerObjectiveLedger(step, ledger, payload.ObjectiveLedger, result.Observations, cfg.CurrentWorkingDirectory, onEvent)
		result.ObjectiveLedger = ledger
		refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
		if hasStructuredProofPlan(payload.ProofPlan) {
			if repaired, reason := repairStructuredProofPlanFromLedger(&payload.ProofPlan, ledger); repaired {
				emitStructuredCommandEvent(onEvent, "structured_proof_plan_repaired", "Proof plan repaired from objective ledger", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": truncateStructuredTimelineValue(reason),
				})
			}
			if err := validateStructuredProofPlan(payload.ProofPlan, ledger); err != nil {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:             step,
					RejectedResponse: truncateStructuredObservation(resp.Content),
					ExitCode:         1,
					Stderr:           "proof_plan invalid: " + err.Error(),
				})
				emitStructuredCommandEvent(onEvent, "structured_proof_plan_rejected", "Proof plan rejected by deterministic validation", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": truncateStructuredTimelineValue(err.Error()),
				})
				continue
			}
			emitStructuredCommandEvent(onEvent, "structured_proof_plan_validated", "Proof plan validated against objective ledger", map[string]string{
				"step":         fmt.Sprintf("%d", step),
				"objective_id": payload.ProofPlan.ObjectiveID,
				"proof_type":   payload.ProofPlan.ProofType,
			})
		}
		emitStructuredCommandEvent(onEvent, "structured_llm_payload_received", "Structured command payload received", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"done":               fmt.Sprintf("%t", payload.Done),
			"ask":                fmt.Sprintf("%t", payload.Ask),
			"tool":               truncateStructuredTimelineValue(payload.Tool),
			"command":            truncateStructuredTimelineValue(payload.Command),
			"pending_objectives": pendingStructuredObjectiveIDs(ledger),
		})
		if repaired, repairResp, repairPayload, repairLedger, repairErr := repairStructuredPayloadBeforeRouting(ctx, step, prompt, client, basePlannerReq, resp, payload, ledger, result.Observations, cfg.CurrentWorkingDirectory, worksiteSurvey, onEvent); repairErr != nil {
			return result, repairErr
		} else if repaired {
			resp = repairResp
			payload = repairPayload
			ledger = repairLedger
			result.ObjectiveLedger = ledger
			refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
		}
		if rejectMixedAskCommandPayload(step, payload, onEvent, &result) {
			continue
		}
		if isPatchToolDelegation(payload) {
			if err := validateStructuredCommandForTaskMode(payload.Command, payload.Patch, worksiteSurvey.TaskMode); err != nil {
				emitStructuredCommandEvent(onEvent, "research_only_mutation_rejected", "Patch delegation rejected by research-only mode", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": err.Error(),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:             step,
					RejectedResponse: truncateStructuredObservation(resp.Content),
					ExitCode:         1,
					Stderr:           "command rejected: " + err.Error(),
				})
				continue
			}
			if err := runStructuredPatchApply(ctx, step, payload.Patch, cfg.CurrentWorkingDirectory, stdout, stderr, onEvent, &result); err != nil {
				return result, err
			}
			continue
		}
		if isShellToolDelegation(payload) {
			if handled, err := runArchitectCodeContentLane(ctx, step, prompt, payload.ToolTask, cfg, worksiteSurvey, stdout, stderr, onEvent, &result); handled || err != nil {
				if err != nil {
					return result, err
				}
				continue
			}
			if cfg.ShellSpecialist == nil {
				emitStructuredCommandEvent(onEvent, "structured_tool_delegation_rejected", "Shell tool delegation rejected", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "shell specialist is not configured",
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "tool delegation rejected: shell specialist is not configured; return a concrete command instead",
				})
				continue
			}
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_started", "Planner delegated shell command selection", map[string]string{
				"step":      fmt.Sprintf("%d", step),
				"tool":      "shell",
				"role":      "shell_execution_specialist",
				"tool_task": truncateStructuredTimelineValue(payload.ToolTask),
			})
			proposal, ok, err := proposeValidatedShellCommand(ctx, step, prompt, payload.ToolTask, cfg, worksiteSurvey, &ledger, onEvent, onAsk, &result)
			if err != nil {
				return result, err
			}
			result.ObjectiveLedger = ledger
			if !ok {
				continue
			}
			if err := runDelegatedShellProposalWithLocalRepair(ctx, step, prompt, payload.ToolTask, proposal, cfg, worksiteSurvey, &ledger, stdout, stderr, onEvent, onAsk, &result); err != nil {
				return result, err
			}
			continue
		}
		if payload.Ask {
			question := strings.TrimSpace(payload.Question)
			command := strings.TrimSpace(payload.Command)
			if structuredQuestionAsksUserToRunCommand(question) {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected_manual_command", "Ask rejected because it asks the user to run an agent command manually", map[string]string{
					"step":     fmt.Sprintf("%d", step),
					"question": truncateStructuredTimelineValue(question),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: models may not ask the user to run normal agent commands manually; return a command for Omnidex to validate or ask a real clarification question",
				})
				continue
			}
			if !hasRealCommandObservation(result.Observations) && command == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected before real command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: no real command observation exists; inspect or try a command first",
				})
				continue
			}
			if latestRealCommandSucceeded(result.Observations) && command == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected after latest command success", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: latest real command succeeded; continue with observed evidence, verify with another command, or finish",
				})
				continue
			}
			previousAnswer, alreadyAnswered := previousUserResponseForQuestion(result.Observations, question)
			if alreadyAnswered {
				emitStructuredCommandEvent(onEvent, "structured_user_input_reused", "Structured loop reused prior user input", map[string]string{
					"step":     fmt.Sprintf("%d", step),
					"question": truncateStructuredTimelineValue(question),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:         step,
					ExitCode:     0,
					Question:     question,
					UserResponse: previousAnswer,
				})
				continue
			}
			if question == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected by structured payload validation", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "empty question with ask=true",
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: empty question with ask=true",
				})
				continue
			}
			if onAsk == nil {
				return result, UserInputRequiredError{Question: question}
			}
			emitStructuredCommandEvent(onEvent, "structured_user_input_requested", "Structured loop requested user input", map[string]string{
				"step":     fmt.Sprintf("%d", step),
				"question": truncateStructuredTimelineValue(question),
			})
			answer, err := onAsk(ctx, question)
			if err != nil {
				if isStructuredUserInputCancelled(ctx, err) {
					markStructuredUserInputCancelled(step, question, onEvent, &result)
				}
				return result, err
			}
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:         step,
				ExitCode:     0,
				Question:     question,
				UserResponse: truncateStructuredObservation(answer),
			})
			emitStructuredCommandEvent(onEvent, "structured_user_input_received", "Structured loop received user input", map[string]string{
				"step": fmt.Sprintf("%d", step),
			})
			continue
		}
		if payload.Done && strings.TrimSpace(payload.Command) != "" && len(pendingStructuredObjectives(ledger)) > 0 && !repeatedSuccessfulStructuredCommand(payload.Command, result.Observations) {
			emitStructuredCommandEvent(onEvent, "structured_done_ignored", "Done flag ignored for non-empty command", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "done=true is only a final validation request when command is empty; executing non-empty command first",
			})
			command := payload.Command
			if err := validateStructuredCommandForRunWithArchitect(command, prompt, payload.ToolTask, payload.Patch, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
				if approved, askErr := requestDependencyInstallApproval(ctx, step, prompt, command, err, cfg.UserAssistanceSpecialist, onAsk, onEvent, &result); askErr != nil {
					return result, askErr
				} else if approved {
					if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
						return result, err
					}
					continue
				}
				emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"command": truncateStructuredTimelineValue(command),
					"reason":  err.Error(),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:             step,
					RejectedCommand:  truncateStructuredObservation(command),
					CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(command, err.Error()),
					ExitCode:         1,
					Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
				})
				continue
			}
			if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
				return result, err
			}
			continue
		}
		if payload.Done {
			if len(pendingStructuredObjectives(ledger)) > 0 {
				gateDecision := ProgressionGate{MaxRecoveryAttempts: 4}.ReviewStep(ProgressionInput{
					Prompt:          prompt,
					WorkingDir:      cfg.CurrentWorkingDirectory,
					WorksiteSurvey:  worksiteSurvey,
					ObjectiveLedger: ledger,
					Observations:    result.Observations,
				})
				if gateDecision.Action != ProgressAllow {
					emitStructuredCommandEvent(onEvent, "progression_gate_rejected_false_done", "Progression gate rejected done=true while blocked objectives remain", map[string]string{
						"step":               fmt.Sprintf("%d", step),
						"pending_objectives": pendingStructuredObjectiveIDs(ledger),
						"action":             string(gateDecision.Action),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:     step,
						ExitCode: 1,
						Stderr:   "progression_gate: done=true rejected before completion validation; blocked recovery or pending objectives require a different action first",
					})
					result.Answer = ""
					if (gateDecision.Action == ProgressForceRecovery || gateDecision.Action == ProgressUseCompletedEvidence) && cfg.ShellSpecialist != nil && worksiteSurvey.TaskMode != TaskModeResearchOnly {
						handled, err := runProgressionGateRecovery(ctx, step, prompt, gateDecision, cfg, worksiteSurvey, stdout, stderr, onEvent, onAsk, &result)
						if err != nil {
							return result, err
						}
						if handled {
							continue
						}
					}
					continue
				}
			}
			if strings.TrimSpace(payload.Command) != "" {
				if latest, ok := latestSuccessfulCommandObservation(result.Observations); ok && latestRealCommandSucceeded(result.Observations) {
					result.Answer = finalStructuredAnswer(payload.Answer, latest)
					ledger = reconcileStructuredObjectiveLedgerFromObservation(step, ledger, latest, onEvent)
					result.ObjectiveLedger = ledger
					refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
					previousLedger := ledger
					if rejectDoneForObjectiveLedger(step, ledger, onEvent, &result) {
						if latestPrematureDoneLoopBlocked(result.Observations) {
							if hasSuccessfulCommandObservation(result.Observations) {
								result.PartialProgress = true
							}
							if result.ExitCode == 0 {
								result.ExitCode = 1
							}
							return result, CommandDecisionExhaustedError{MaxSteps: step}
						}
						continue
					} else if !deterministicCompletionEnforcerAcceptsDone(prompt, ledger, result.Observations) {
						result.ObjectiveLedger = ledger
						refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
						rejectDoneForValidator(step, onEvent, &result)
						continue
					}
					ledger = reconcileStructuredObjectiveLedgerForDone(step, ledger, latest, onEvent)
					ledger = enforcePostWriteValidationBeforeCompletion(step, prompt, previousLedger, ledger, result.Observations, onEvent, &result)
					ledger = enforceNoEmptyProjectFilesBeforeCompletion(step, prompt, cfg.CurrentWorkingDirectory, ledger, result.Observations, onEvent, &result)
					result.ObjectiveLedger = ledger
					refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
					if rejectArtifactValidationGate(step, prompt, cfg.CurrentWorkingDirectory, ledger, result.Observations, onEvent, &result) {
						continue
					}
					if rejectDoneForObjectiveLedger(step, ledger, onEvent, &result) {
						if latestPrematureDoneLoopBlocked(result.Observations) {
							if hasSuccessfulCommandObservation(result.Observations) {
								result.PartialProgress = true
							}
							if result.ExitCode == 0 {
								result.ExitCode = 1
							}
							return result, CommandDecisionExhaustedError{MaxSteps: step}
						}
						continue
					}
					if rejectDoneForFinalAnswer(step, prompt, result.Answer, onEvent, &result) {
						continue
					}
					if rejectTypedFinalGate(step, cfg.CurrentWorkingDirectory, onEvent, &result) {
						continue
					}
					if typedWorkQueuePassedForCompletion(&result) && hasImplementationArchitectProgress(result.Observations) {
						accepted, evalErr := runFinalBroadEvaluatorAfterTypedCompletion(ctx, step, prompt, evaluator, evaluatorThreshold, cfg, worksiteSurvey, ledger, result.Observations, result.Answer, onEvent, &result)
						if evalErr != nil {
							return result, evalErr
						}
						if !accepted {
							continue
						}
						emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_typed_final_gate", "Typed recursive work queue accepted completion with required evidence", map[string]string{
							"step":    fmt.Sprintf("%d", step),
							"command": truncateStructuredTimelineValue(payload.Command),
							"answer":  truncateStructuredTimelineValue(result.Answer),
						})
						return result, nil
					}
					if cfg.CompletionChecker != nil {
						if rejectCompletionCheckerWithoutTypedWorkQueue(step, onEvent, &result) {
							continue
						}
						checkResult := runCompletionCheckDetailed(ctx, step, prompt, cfg.CurrentWorkingDirectory, ledger, minimalContext, result.Observations, result.Answer, cfg.CompletionChecker, worksiteSurvey, onEvent)
						if checkResult.Err != nil {
							return result, checkResult.Err
						}
						ledger = checkResult.Ledger
						result.ObjectiveLedger = ledger
						refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
						if !checkResult.Accepted {
							rejectDoneForValidator(step, onEvent, &result)
							if handled, err := repairRejectedDoneWithPlanner(ctx, step, prompt, client, basePlannerReq, resp, payload, checkResult, cfg, worksiteSurvey, stdout, stderr, onEvent, onAsk, &ledger, &result); err != nil {
								return result, err
							} else if handled {
								continue
							}
							continue
						}
					}
					if rejectCompletionCheckerWithoutTypedWorkQueue(step, onEvent, &result) {
						continue
					}
					emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_done_request", "Completion validator accepted evidence after planner requested final validation", map[string]string{
						"step":    fmt.Sprintf("%d", step),
						"command": truncateStructuredTimelineValue(payload.Command),
						"answer":  truncateStructuredTimelineValue(result.Answer),
						"reason":  "non-empty command ignored because successful command evidence already exists",
					})
					return result, nil
				}
				emitStructuredCommandEvent(onEvent, "structured_done_ignored", "Done flag ignored for non-empty command", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "done=true requires an empty command; validating non-empty command instead",
				})
				command := payload.Command
				if err := validateStructuredCommandForRunWithArchitect(command, prompt, payload.ToolTask, payload.Patch, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
					emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
						"step":    fmt.Sprintf("%d", step),
						"command": truncateStructuredTimelineValue(command),
						"reason":  err.Error(),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:             step,
						RejectedCommand:  truncateStructuredObservation(command),
						CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(command, err.Error()),
						ExitCode:         1,
						Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
					})
					continue
				}
				if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
					return result, err
				}
				continue
			}
			if !hasRealCommandObservation(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected before real command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: no real command observation exists",
				})
				continue
			}
			if !hasSuccessfulCommandObservation(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected before successful command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: no successful command observation exists",
				})
				continue
			}
			if !latestRealCommandSucceeded(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected after latest command failure", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: latest real command failed; try a different command or source",
				})
				continue
			}
			latest, _ := latestSuccessfulCommandObservation(result.Observations)
			result.Answer = finalStructuredAnswer(payload.Answer, latest)
			if len(selectedRecipes) > 0 {
				var recipeObservations []StructuredCommandObservation
				ledger, recipeObservations = runSelectedRecipeCompletionProbes(ctx, step, cfg.CurrentWorkingDirectory, ledger, selectedRecipes, onEvent)
				result.Observations = append(result.Observations, recipeObservations...)
			}
			ledger = reconcileStructuredObjectiveLedgerFromObservation(step, ledger, latest, onEvent)
			result.ObjectiveLedger = ledger
			refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
			previousLedger := ledger
			if rejectDoneForObjectiveLedger(step, ledger, onEvent, &result) {
				if latestPrematureDoneLoopBlocked(result.Observations) {
					if hasSuccessfulCommandObservation(result.Observations) {
						result.PartialProgress = true
					}
					if result.ExitCode == 0 {
						result.ExitCode = 1
					}
					return result, CommandDecisionExhaustedError{MaxSteps: step}
				}
				continue
			} else if !deterministicCompletionEnforcerAcceptsDone(prompt, ledger, result.Observations) {
				result.ObjectiveLedger = ledger
				refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
				rejectDoneForValidator(step, onEvent, &result)
				continue
			}
			ledger = reconcileStructuredObjectiveLedgerForDone(step, ledger, latest, onEvent)
			ledger = enforcePostWriteValidationBeforeCompletion(step, prompt, previousLedger, ledger, result.Observations, onEvent, &result)
			ledger = enforceNoEmptyProjectFilesBeforeCompletion(step, prompt, cfg.CurrentWorkingDirectory, ledger, result.Observations, onEvent, &result)
			result.ObjectiveLedger = ledger
			refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
			if rejectArtifactValidationGate(step, prompt, cfg.CurrentWorkingDirectory, ledger, result.Observations, onEvent, &result) {
				continue
			}
			if rejectDoneForObjectiveLedger(step, ledger, onEvent, &result) {
				if latestPrematureDoneLoopBlocked(result.Observations) {
					if hasSuccessfulCommandObservation(result.Observations) {
						result.PartialProgress = true
					}
					if result.ExitCode == 0 {
						result.ExitCode = 1
					}
					return result, CommandDecisionExhaustedError{MaxSteps: step}
				}
				continue
			}
			if rejectDoneForFinalAnswer(step, prompt, result.Answer, onEvent, &result) {
				continue
			}
			if rejectTypedFinalGate(step, cfg.CurrentWorkingDirectory, onEvent, &result) {
				continue
			}
			if typedWorkQueuePassedForCompletion(&result) && hasImplementationArchitectProgress(result.Observations) {
				accepted, evalErr := runFinalBroadEvaluatorAfterTypedCompletion(ctx, step, prompt, evaluator, evaluatorThreshold, cfg, worksiteSurvey, ledger, result.Observations, result.Answer, onEvent, &result)
				if evalErr != nil {
					return result, evalErr
				}
				if !accepted {
					continue
				}
				emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_typed_final_gate", "Typed recursive work queue accepted completion with required evidence", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"answer": truncateStructuredTimelineValue(result.Answer),
				})
				return result, nil
			}
			if cfg.CompletionChecker != nil {
				if rejectCompletionCheckerWithoutTypedWorkQueue(step, onEvent, &result) {
					continue
				}
				checkResult := runCompletionCheckDetailed(ctx, step, prompt, cfg.CurrentWorkingDirectory, ledger, minimalContext, result.Observations, result.Answer, cfg.CompletionChecker, worksiteSurvey, onEvent)
				if checkResult.Err != nil {
					return result, checkResult.Err
				}
				ledger = checkResult.Ledger
				result.ObjectiveLedger = ledger
				refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
				if !checkResult.Accepted {
					rejectDoneForValidator(step, onEvent, &result)
					if handled, err := repairRejectedDoneWithPlanner(ctx, step, prompt, client, basePlannerReq, resp, payload, checkResult, cfg, worksiteSurvey, stdout, stderr, onEvent, onAsk, &ledger, &result); err != nil {
						return result, err
					} else if handled {
						continue
					}
					continue
				}
			}
			if rejectCompletionCheckerWithoutTypedWorkQueue(step, onEvent, &result) {
				continue
			}
			emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_done_request", "Completion validator accepted evidence after planner requested final validation", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"answer": truncateStructuredTimelineValue(result.Answer),
			})
			return result, nil
		}
		if strings.TrimSpace(payload.Command) == "" {
			if cfg.ShellSpecialist != nil {
				toolTask := strings.TrimSpace(payload.ToolTask)
				if toolTask == "" {
					toolTask = prompt
				}
				handled, err := runDelegatedShellSpecialist(ctx, step, prompt, toolTask, cfg, worksiteSurvey, stdout, stderr, onEvent, onAsk, &result)
				if err != nil {
					return result, err
				}
				if handled {
					continue
				}
			}
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "empty command with done=false",
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "command rejected: empty command with done=false; choose an evidence-gathering command from tool_inventory",
			})
			continue
		}
		command := payload.Command
		if rejectedByChild, updatedJobs := rejectCommandRepeatedByActiveChildJob(step, command, result.ChildJobs, onEvent, &result); rejectedByChild {
			result.ChildJobs = updatedJobs
			refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
			continue
		}
		if !payload.Done && repeatedSuccessfulStructuredCommand(command, result.Observations) {
			var latest *StructuredCommandObservation
			if len(result.Observations) > 0 {
				latest = &result.Observations[len(result.Observations)-1]
			}
			reconciledSuccess := RunSuccessReconciliation(SuccessReconciliationInput{
				LatestObservation: latest,
				ObjectiveLedger:   ledger,
				WorkQueue:         result.WorkItems,
				ChildJobs:         result.ChildJobs,
				WorkingDirectory:  cfg.CurrentWorkingDirectory,
				Observations:      result.Observations,
			})
			ledger = reconciledSuccess.ObjectiveLedger
			result.ObjectiveLedger = ledger
			result.WorkItems = reconciledSuccess.WorkQueue
			result.ChildJobs = reconciledSuccess.ChildJobs
			emitSuccessReconciliationEvents(onEvent, reconciledSuccess.Events)
			if len(reconciledSuccess.SatisfiedObjectives) > 0 {
				continue
			}
			if handleStructuredRepeatedCommandValidation(step, command, errRepeatedSuccessfulStructuredCommand, &ledger, onEvent, &result) {
				continue
			}
		}
		if err := validateStructuredCommandForRunWithArchitect(command, prompt, payload.ToolTask, payload.Patch, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
			if approved, askErr := requestDependencyInstallApproval(ctx, step, prompt, command, err, cfg.UserAssistanceSpecialist, onAsk, onEvent, &result); askErr != nil {
				return result, askErr
			} else if approved {
				if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
					return result, err
				}
				continue
			}
			if handleStructuredRepeatedCommandValidation(step, command, err, &ledger, onEvent, &result) {
				gate := ProgressionGate{MaxRecoveryAttempts: 4}
				decision := gate.ReviewStep(ProgressionInput{
					Prompt:          prompt,
					WorkingDir:      cfg.CurrentWorkingDirectory,
					WorksiteSurvey:  worksiteSurvey,
					ObjectiveLedger: result.ObjectiveLedger,
					Observations:    result.Observations,
				})
				if decision.Action == ProgressFailWithEvidence {
					emitStructuredCommandEvent(onEvent, "progression_gate_failed", "Progression gate exhausted recovery routes", map[string]string{
						"step":   fmt.Sprintf("%d", step),
						"reason": decision.Reason,
					})
					result.PartialProgress = hasSuccessfulCommandObservation(result.Observations) || len(result.Observations) > 0
					if result.ExitCode == 0 {
						result.ExitCode = 1
					}
					return result, CommandDecisionExhaustedError{MaxSteps: step}
				}
				if (decision.Action == ProgressForceRecovery || decision.Action == ProgressUseCompletedEvidence) && cfg.ShellSpecialist != nil && worksiteSurvey.TaskMode != TaskModeResearchOnly {
					handled, recoverErr := runProgressionGateRecovery(ctx, step, prompt, decision, cfg, worksiteSurvey, stdout, stderr, onEvent, onAsk, &result)
					if recoverErr != nil {
						return result, recoverErr
					}
					if handled {
						continue
					}
				}
				continue
			}
			if isPlaceholderPathValidationError(err) {
				emitStructuredCommandEvent(onEvent, "structured_command_rejected_placeholder_path", "Command rejected because it used a literal placeholder project path", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"command": truncateStructuredTimelineValue(command),
					"reason":  err.Error(),
				})
			}
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(command),
				"reason":  err.Error(),
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:             step,
				RejectedCommand:  truncateStructuredObservation(command),
				CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(command, err.Error()),
				ExitCode:         1,
				Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
			})
			continue
		}

		if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
			return result, err
		}
	}

	return result, finalizeStructuredLoopExhaustion(ctx, prompt, cfg, worksiteSurvey, ledger, onEvent, &result)
}
