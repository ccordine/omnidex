export type ModelFieldDefinition = {
  key: string;
  label: string;
  description: string;
  env_keys: string[];
  options?: string[];
  value: string;
};

export type ResolvedModelConfig = {
  resolved: Record<string, string>;
  source: "env" | "project" | "card" | string;
  fields: ModelFieldDefinition[];
};

export type ModelDefaultsResponse = {
  env_defaults: Record<string, string>;
  fields: ModelFieldDefinition[];
  resolved?: ResolvedModelConfig;
};
