export interface Layer {
  id: string;
  namespace: string;
  name: string;
  salt: string;
  bucket_size: number;
  active: boolean;
}

export interface Criteria {
  require_registered: boolean;
  allowed_countries: string[];
  allowed_plans: string[];
}

export interface Experiment {
  id: string;
  layer_id: string;
  name: string;
  start_bucket: number;
  end_bucket: number;
  salt: string;
  active: boolean;
  criteria: Criteria;
}

export interface Variant {
  id: string;
  experiment_id: string;
  name: string;
  start_bucket: number;
  end_bucket: number;
  active: boolean;
  payload?: Record<string, unknown>;
}

export interface Whitelist {
  id: string;
  user_id: string;
  layer_id: string;
  experiment_id: string;
  variant_id: string;
  reason: string;
  active: boolean;
}

export interface ExperimentConfig {
  layers: Layer[];
  experiments: Experiment[];
  variants: Variant[];
  whitelists: Whitelist[];
}

export interface Subject {
  user_id: string;
  registered: boolean;
  country: string;
  plan: string;
  source: string;
}

export interface TraceStep {
  name: string;
  passed: boolean;
  detail: string;
  context?: Record<string, unknown>;
}

export interface Decision {
  user_id: string;
  layer?: Layer;
  bucket: number;
  hash_input: string;
  experiment?: Experiment;
  variant?: Variant;
  variant_hash_input?: string;
  variant_bucket?: number;
  status: string;
  reject_reason?: string;
  whitelist?: Whitelist;
  path: string[];
  steps: TraceStep[];
}

export interface DecideResponse {
  decision: Decision;
  recorded: boolean;
  exposed: boolean;
}

export interface SimulationResponse {
  count: number;
  groups: Record<string, number>;
  rejections: Record<string, number>;
  percentages: Record<string, number>;
}

export interface Stats {
  assignments: Record<string, number>;
  exposures: Record<string, number>;
  rejections: Record<string, number>;
  whitelist_assignments: number;
  assignment_total: number;
  exposure_total: number;
}
