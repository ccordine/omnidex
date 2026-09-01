export type ProjectRecord = {
  id: number;
  name: string;
  location: string;
  description: string;
  last_seen_at: string;
  created_at: string;
  updated_at: string;
  job_count?: number;
  card_count?: number;
};
