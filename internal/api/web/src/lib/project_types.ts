export type ProjectRecord = {
  id: number;
  name: string;
  location: string;
  description?: string;
  recipe_id?: string;
  recipe?: Record<string, unknown>;
  project_state?: string;
  settings?: Record<string, unknown>;
  model_config?: Record<string, string>;
  agent_config?: Record<string, string>;
  last_seen_at: string;
  created_at: string;
  updated_at: string;
  job_count?: number;
  card_count?: number;
  is_active?: boolean;
};

export type RecipeCatalogItem = {
  id: string;
  description: string;
  operation?: string;
  objectives?: Array<{ id: string; description: string; depends_on?: string[] }>;
  allowed_commands?: string[];
  evidence_required?: string[];
  completion_checks?: string[];
};

export type BrowseEntry = {
  name: string;
  path: string;
  is_dir: boolean;
};

export type BrowseResponse = {
  path: string;
  parent: string;
  entries: BrowseEntry[];
};

export type WorkspaceResponse = {
  active_project_id: number;
  project?: ProjectRecord;
};

export type ProjectMapSummary = {
  exists: boolean;
  map_path: string;
  relative_map_path?: string;
  generated_at?: string;
  revision?: string;
  workspace_id?: string;
  root?: string;
  file_count: number;
  module_count: number;
  stale_file_count: number;
  languages: Array<{ language: string; files: number; bytes?: number }>;
  modules: Array<{
    path: string;
    purpose?: string;
    important_files?: string[];
    confidence?: number;
    stale?: boolean;
    responsibilities?: string[];
  }>;
  entrypoints: Array<{ path: string; kind?: string; reason?: string }>;
  commands: Array<{ name: string; command: string; source?: string }>;
  tests: string[];
  risks: Array<{ area: string; risk: string; reason?: string }>;
  manifests?: string[];
  open_questions?: string[];
  files_preview?: Array<{ path: string; language?: string; module?: string; purpose?: string; stale?: boolean }>;
  tree_preview?: string;
  message?: string;
};

export type DebuggerCreatedCard = {
  id: string;
  title: string;
  severity?: string;
};

export type DebuggerLastRun = {
  job_id?: number;
  project_id?: number;
  agent_system?: string;
  model?: string;
  status?: string;
  summary?: string;
  findings_count?: number;
  cards_created?: DebuggerCreatedCard[];
  suggestions?: string[];
  started_at?: string;
  completed_at?: string;
  error?: string;
};
