export type AgentFieldDefinition = {
  key: string;
  label: string;
  description: string;
  env_keys: string[];
  options?: string[];
  value: string;
};

export type ResolvedAgentConfig = {
  resolved: Record<string, string>;
  source: "env" | "project" | "card" | string;
  fields: AgentFieldDefinition[];
  system: string;
  strict: boolean;
  external: boolean;
};
