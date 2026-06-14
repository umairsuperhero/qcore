import type {
  CoreHealth,
  Subscriber,
  SubscriberListResponse,
  RANConfig,
  RANReconcileResult,
  SimulatorStatus,
  DiagnosticResult,
  ScenarioRecord,
  ScenarioRunResult,
  ScenarioSummary,
} from "./types";

async function jsonGet<T>(path: string): Promise<T> {
  const r = await fetch(path);
  if (!r.ok) {
    throw new Error(`${path}: ${r.status} ${r.statusText}`);
  }
  return r.json();
}

async function jsonSend<T>(method: string, path: string, body?: unknown): Promise<T> {
  const r = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!r.ok) {
    const text = await r.text();
    throw new Error(`${path}: ${r.status} — ${text}`);
  }
  // Some endpoints return 204 No Content
  if (r.status === 204) return {} as T;
  return r.json();
}

export const api = {
  health: () => jsonGet<CoreHealth>("/api/health"),
  ranConfig: () => jsonGet<RANConfig>("/api/ran-config"),
  reconcileRANConfig: (gnbYaml: string, ueYaml: string) =>
    jsonSend<RANReconcileResult>("POST", "/api/ran-config/reconcile", {
      gnb_yaml: gnbYaml,
      ue_yaml: ueYaml,
    }),

  listSubscribers: (page = 1, limit = 50) =>
    jsonGet<SubscriberListResponse>(
      `/api/subscribers?page=${page}&limit=${limit}`,
    ),
  createSubscriber: (sub: Subscriber) =>
    jsonSend<{ data: Subscriber }>("POST", "/api/subscribers", sub),
  deleteSubscriber: (imsi: string) =>
    jsonSend<unknown>("DELETE", `/api/subscribers/${imsi}`),

  simulatorStatus: () => jsonGet<SimulatorStatus>("/api/simulator/status"),
  simulatorStart: (mode: string = "4g") => jsonSend<SimulatorStatus>("POST", "/api/simulator/start", { mode }),
  simulatorStop: () => jsonSend<SimulatorStatus>("POST", "/api/simulator/stop"),
  simulatorInject: (mode: string = "4g", scenario: string) =>
    jsonSend<SimulatorStatus>("POST", `/api/simulator/inject/${scenario}`, { mode }),
  simulatorCustom: (yamlContent: string) =>
    fetch("/api/simulator/custom", {
      method: "POST",
      headers: { "Content-Type": "application/yaml" },
      body: yamlContent,
    }).then((r) => {
      if (!r.ok) throw new Error("Failed to run custom scenario");
      return r.json() as Promise<SimulatorStatus>;
    }),
  listScenarios: () => jsonGet<ScenarioSummary[]>("/api/scenarios"),
  getScenario: (name: string) => jsonGet<ScenarioRecord>(`/api/scenarios/${encodeURIComponent(name)}`),
  saveScenario: (yamlContent: string) =>
    fetch("/api/scenarios", {
      method: "POST",
      headers: { "Content-Type": "application/yaml" },
      body: yamlContent,
    }).then(async (r) => {
      if (!r.ok) throw new Error(await r.text());
      return r.json() as Promise<ScenarioRecord>;
    }),
  deleteScenario: (name: string) =>
    jsonSend<unknown>("DELETE", `/api/scenarios/${encodeURIComponent(name)}`),
  runScenario: (name: string) =>
    jsonSend<ScenarioRunResult>("POST", `/api/scenarios/${encodeURIComponent(name)}/run`),

  diagnoseJourney: (journeyID: string) =>
    jsonGet<DiagnosticResult>(`/api/diagnostics/journey/${journeyID}`),
};
