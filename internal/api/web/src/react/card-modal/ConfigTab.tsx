import { useEffect, useState } from "react";
import { patchScrumCard } from "../../lib/scrum_api";
import type { ScrumConfigField } from "../../lib/scrum_types";
import { ActionButton, Panel, Select, TextInput } from "./common";
import type { CardModalChildProps } from "./types";

function initialValues(fields: ScrumConfigField[] = [], overrides: Record<string, string> = {}) {
  const out: Record<string, string> = {};
  for (const field of fields) {
    if (overrides[field.key] != null) out[field.key] = overrides[field.key];
  }
  for (const [key, value] of Object.entries(overrides)) out[key] = value;
  return out;
}

function inheritLabel(field: ScrumConfigField) {
  const inherited = field.value?.trim();
  return inherited ? `Inherit (${inherited})` : "Inherit default";
}

function ConfigFields({ fields, values, onChange }: { fields: ScrumConfigField[]; values: Record<string, string>; onChange: (key: string, value: string) => void }) {
  if (fields.length === 0) return <p className="text-sm text-zinc-500">No fields available.</p>;
  return (
    <div className="grid gap-3">
      {fields.map((field) => (
        <label key={field.key} className="grid gap-1 text-sm">
          <span className="font-medium text-zinc-200">{field.label || field.key}</span>
          {field.description ? <span className="text-xs text-zinc-500">{field.description}</span> : null}
          {field.options?.length ? (
            <Select value={values[field.key] ?? ""} onChange={(event) => onChange(field.key, event.target.value)}>
              <option value="">{inheritLabel(field)}</option>
              {field.options.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </Select>
          ) : (
            <TextInput value={values[field.key] ?? ""} placeholder={field.value || "Inherit default"} onChange={(event) => onChange(field.key, event.target.value)} />
          )}
        </label>
      ))}
    </div>
  );
}

function compact(values: Record<string, string>) {
  return Object.fromEntries(Object.entries(values).filter(([, value]) => value.trim() !== ""));
}

export function ConfigTab({ context, projectID, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const modelFields = context.model_fields ?? [];
  const agentFields = context.agent_fields ?? [];
  const [modelValues, setModelValues] = useState<Record<string, string>>({});
  const [agentValues, setAgentValues] = useState<Record<string, string>>({});

  useEffect(() => {
    setModelValues(initialValues(modelFields, context.model_overrides));
    setAgentValues(initialValues(agentFields, context.agent_overrides));
  }, [card.id, card.updated_at, context.model_overrides, context.agent_overrides]);

  async function saveModel(values = modelValues) {
    const updated = await runMutation("Saving model settings", () => patchScrumCard(card.id, { model_config: compact(values) }, projectID));
    if (updated) onCardUpdated(updated, { reloadContext: true });
  }

  async function saveAgent(values = agentValues) {
    const updated = await runMutation("Saving agent settings", () => patchScrumCard(card.id, { agent_config: compact(values) }, projectID));
    if (updated) onCardUpdated(updated, { reloadContext: true });
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Panel title="Model Overrides" aside={<span className="text-xs text-zinc-500">{context.model_source || "default"}</span>}>
        <div className="space-y-3">
          <ConfigFields fields={modelFields} values={modelValues} onChange={(key, value) => setModelValues((current) => ({ ...current, [key]: value }))} />
          <div className="flex gap-2">
            <ActionButton tone="primary" onClick={() => void saveModel()}>Save model</ActionButton>
            <ActionButton onClick={() => { setModelValues({}); void saveModel({}); }}>Clear</ActionButton>
          </div>
        </div>
      </Panel>

      <Panel title="Agent Overrides" aside={<span className="text-xs text-zinc-500">{context.agent_system || "omnidex"}</span>}>
        <div className="space-y-3">
          <div className="flex flex-wrap gap-2">
            {["omnidex", "codex", "cursor"].map((system) => (
              <ActionButton
                key={system}
                tone={system === (agentValues.agent_system || context.agent_system) ? "primary" : "default"}
                onClick={() => {
                  const next = { ...agentValues, agent_system: system, ...(system === "codex" || system === "cursor" ? { agent_strict: "true" } : {}) };
                  setAgentValues(next);
                  void saveAgent(next);
                }}
              >
                Use {system}
              </ActionButton>
            ))}
          </div>
          <ConfigFields fields={agentFields} values={agentValues} onChange={(key, value) => setAgentValues((current) => ({ ...current, [key]: value }))} />
          <div className="flex gap-2">
            <ActionButton tone="primary" onClick={() => void saveAgent()}>Save agent</ActionButton>
            <ActionButton onClick={() => { setAgentValues({}); void saveAgent({}); }}>Clear</ActionButton>
          </div>
        </div>
      </Panel>
    </div>
  );
}
