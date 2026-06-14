// Mirrors the Go types in pkg/dashboard/*.go and pkg/events/event.go.
// Keeping these in sync is manual today; a code-gen step is Phase D.

export interface NFStatus {
  name: string;
  url: string;
  reachable: boolean;
  status?: string;
  latency_ms: number;
  error?: string;
}

export interface CoreHealth {
  all_up: boolean;
  ready_for_use: boolean;
  checked_at: string;
  nfs: NFStatus[];
}

export interface Subscriber {
  imsi: string;
  ki: string;
  opc: string;
  amf?: string;
  sqn?: string;
  apn?: string;
  status?: number;
}

export interface SubscriberListResponse {
  data: Subscriber[];
  page: number;
  limit: number;
  total: number;
}

export type EventCategory =
  | "signaling_rx"
  | "signaling_tx"
  | "state_transition"
  | "config_change"
  | "error";

export type EventSeverity = "info" | "warn" | "error";

export interface QEvent {
  id: string;
  journey_id?: string;
  timestamp: string;
  nf: string;
  category: EventCategory;
  severity: EventSeverity;
  protocol?: string;
  message: string;
  payload?: Record<string, unknown>;
}

export type SimulatorState = "idle" | "running" | "success" | "failed";

export interface SimulatorStatus {
  state: SimulatorState;
  last_journey?: string;
  last_scenario?: string;
  last_error?: string;
  failed_step?: string;
  mode?: string;
  started_at?: string;
  ended_at?: string;
}

export interface RANConfig {
  plmn: string;
  tac: number;
  mme_address: string;
  mme_s1ap_port: number;
  mme_code: number;
  mme_group_id: number;
  amf_address: string;
  amf_ngap_port: number;
  amf_plmn: string;
  amf_tac: number;
  serving_network_name: string;
  spgw_gtpu_address: string;
  spgw_gtpu_port: number;
  ue_pool: string;
  sample_subscriber: {
    imsi: string;
    ki: string;
    opc: string;
    amf: string;
    apn?: string;
  };
  config_snippet: Record<string, string>;
}

export interface RANConfigMismatch {
  field: string;
  ran_value: string;
  core_value: string;
  cause: string;
  fix: string;
  severity: "error" | "warn" | string;
}

export interface RANReconcileResult {
  ok: boolean;
  summary: string;
  mismatches: RANConfigMismatch[] | null;
  normalized: Record<string, unknown>;
  core?: Record<string, string>;
}

export interface DiagnosticResult {
  Matched: boolean;
  Explanation: string;
  RootCause: string;
  Fix: string;
}
