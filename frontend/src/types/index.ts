export interface HTTPExchange {
  id: number;
  log_id: string;
  direction: 'downstream' | 'upstream';
  phase: 'request' | 'response';
  timestamp: string;
  method?: string;
  url?: string;
  path?: string;
  status_code?: number;
  headers?: string;
  body?: string;
  duration_ms?: number;
  error?: string;
  raw_stream?: string;
}

export interface RequestLog {
  id: string;
  created_at: string;
  session_id: string;
  model: string;
  mapped_model: string;
  stream: boolean;
  status: string;
  error_msg: string;
  downstream_method: string;
  downstream_path: string;
  downstream_req: string;
  downstream_resp: string;
  upstream_req: string;
  upstream_resp: string;
  upstream_status: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  ttft_ms: number;
  upstream_ms: number;
  downstream_ms: number;
  canonical_record?: boolean;
  ingress_protocol?: string;
  ingress_endpoint?: string;
  pre_policy_request?: string;
  post_policy_request?: string;
  session_snapshot?: string;
  execution_sidecar?: string;
  exchanges?: HTTPExchange[];
}

export interface LogListResult {
  items: RequestLog[];
  total: number;
  page: number;
  limit: number;
}

export interface ModelMapping {
  id: number;
  priority: number;
  name: string;
  pattern: string;
  target: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface PolicyMatch {
  protocol?: string;
  requested_model?: string;
  stream?: boolean;
  has_tools?: boolean;
  has_reasoning?: boolean;
  session_present?: boolean;
  client_name?: string;
  ingress_tag?: string;
}

export interface PolicyActions {
  rewrite_model?: string;
  set_reasoning?: boolean;
  allow_tools?: boolean;
  add_tags?: string[];
}

export interface PolicyRule {
  id: number;
  priority: number;
  name: string;
  enabled: boolean;
  match: PolicyMatch;
  actions: PolicyActions;
  source: string;
  created_at: string;
  updated_at: string;
}

export interface PolicyTestInput {
  protocol: string;
  requested_model: string;
  stream: boolean;
  has_tools: boolean;
  has_reasoning: boolean;
  session_present: boolean;
  client_name?: string;
  ingress_tag?: string;
}

export interface PolicyTestResult {
  matched: boolean;
  effective_actions: PolicyActions;
  matched_rules: Array<{
    id: number;
    name: string;
    priority: number;
    applied: PolicyActions;
    suppressed?: string[];
  }>;
}

export interface DashboardStats {
  total_requests: number;
  success_rate: number;
  avg_ttft_ms: number;
  total_tokens: number;
}

export interface TimeSeriesPoint {
  time: string;
  rate?: number;
  prompt?: number;
  completion?: number;
}

export interface ModelDistPoint {
  model: string;
  count: number;
}

export interface DashboardData {
  stats: DashboardStats;
  success_rate_series: TimeSeriesPoint[];
  token_series: TimeSeriesPoint[];
  model_distribution: ModelDistPoint[];
}

export interface AdminStatus {
  loaded: boolean;
  has_credentials: boolean;
  source: string;
  loaded_at: string;
  token_expired?: boolean;
}

export interface ModelStatus {
  fetched_at: string;
  cached: boolean;
  count: number;
  last_error?: string;
}

export interface OverviewLatencyStats {
  avg_ms: number;
  p50_ms: number;
  p95_ms: number;
  max_ms: number;
  sample_count: number;
}

export interface OverviewData {
  healthy: boolean;
  generated_at: string;
  credential: AdminStatus;
  models: ModelStatus;
  session_count: number;
  token_stats: {
    today: number;
    week: number;
    total: number;
  };
  dashboard: DashboardData;
  latency: OverviewLatencyStats;
  recent_requests: RequestLog[];
  available_models: Array<{
    id: string;
    object: string;
    owned_by: string;
  }>;
  settings: Record<string, string>;
}

export interface AdminModelsResponse {
  items: Array<{
    id: string;
    object: string;
    owned_by: string;
  }>;
  status: ModelStatus;
}

export interface AccountData {
  credential: {
    cos_y_key: string;
    encrypt_user_info: string;
    user_id: string;
    machine_id: string;
    loaded_at: string;
  };
  status: {
    loaded: boolean;
    has_credentials: boolean;
    source: string;
    loaded_at: string;
  };
  token_stats: {
    today: number;
    week: number;
    total: number;
  };
}

export type BootstrapMethod = 'auto' | 'ws' | 'remote_callback';

export type BootstrapStatus =
  | 'running'
  | 'awaiting_callback'
  | 'deriving'
  | 'completed'
  | 'error'
  | 'cancelled';

export interface BootstrapResponse {
  id: string;
  status: BootstrapStatus | string;
  method: BootstrapMethod | '';
  phase?: string;
  auth_url?: string;
  error?: string;
  started_at: string;
  expires_at?: string;
}

export type Theme = 'light' | 'dark';
