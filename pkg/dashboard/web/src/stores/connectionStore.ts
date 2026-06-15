import { create } from "zustand";
import { api } from "../api/client";
import type { QEvent, DiagnosticResult, RANConfig, ScenarioRunResult } from "../api/types";

export type GNBState = "waiting" | "connected" | "failed";
export type ProtocolMode = "4g" | "5g";

export interface GNBConnectionData {
  state: GNBState;
  amfAddress: string;
  configuredPlmn: string;
  configuredTac: string;
  gnbName?: string;
  negotiatedPlmn?: string;
  negotiatedTac?: string;
  negotiatedSlice?: string;
  failReason?: string;
  sentPlmn?: string;
  fixGnb?: string;
  fixQCore?: string;
}

export interface TraceStreamState {
  events: QEvent[];
  streaming: boolean;
  activeScenario: string | null;
  mode: ProtocolMode;
  diagnostic: DiagnosticResult | null;
  journeyId: string | null;
  error: string | null;
}

interface ConnectionStore {
  // Config & Status
  loading: boolean;
  config: RANConfig | null;
  connection: GNBConnectionData;
  mode: ProtocolMode;

  // Trace Stream State
  traceState: TraceStreamState;

  // Actions
  fetchConfig: () => Promise<void>;
  initEventSource: () => void;
  closeEventSource: () => void;
  resetToLive: () => void;
  startSimulator: (scenario?: string) => Promise<void>;
  runSavedScenario: (name: string) => Promise<ScenarioRunResult>;
  clearTrace: () => void;
  setMode: (mode: ProtocolMode) => void;
}

let esInstance: EventSource | null = null;

function endpointAddress(address: string, port: number, fallbackPort: number) {
  const host =
    address.startsWith("<") || address === "0.0.0.0"
      ? window.location.hostname || "192.168.1.50"
      : address;
  return `${host}:${port || fallbackPort}`;
}

function formatPlmn(plmn: string) {
  if (plmn.length >= 5) {
    return `${plmn.slice(0, 3)} / ${plmn.slice(3)}`;
  }
  return plmn;
}

function connectionFieldsForMode(cfg: RANConfig, mode: ProtocolMode) {
  if (mode === "4g") {
    return {
      amfAddress: endpointAddress(cfg.mme_address, cfg.mme_s1ap_port, 36412),
      configuredPlmn: formatPlmn(cfg.plmn),
      configuredTac: String(cfg.tac),
    };
  }

  return {
    amfAddress: endpointAddress(cfg.amf_address || cfg.mme_address, cfg.amf_ngap_port, 38412),
    configuredPlmn: formatPlmn(cfg.amf_plmn || cfg.plmn),
    configuredTac: String(cfg.amf_tac || cfg.tac),
  };
}

export const useConnectionStore = create<ConnectionStore>((set, get) => ({
  loading: true,
  config: null,
  mode: "5g",
  connection: {
    state: "waiting",
    amfAddress: "Loading...",
    configuredPlmn: "...",
    configuredTac: "...",
  },

  traceState: {
    events: [],
    streaming: false,
    activeScenario: null,
    mode: "5g",
    diagnostic: null,
    journeyId: null,
    error: null,
  },

  fetchConfig: async () => {
    try {
      const cfg = await api.ranConfig();
      const mode = get().mode;
      const fields = connectionFieldsForMode(cfg, mode);

      set((state) => {
        return {
          config: cfg,
          loading: false,
          connection: {
            ...state.connection,
            ...fields,
          },
        };
      });
    } catch (err) {
      console.error("Failed to load RAN config:", err);
      set({ loading: false });
    }
  },

  initEventSource: () => {
    if (esInstance) return; // Already initialized

    const es = new EventSource("/api/events/stream");
    esInstance = es;

    es.onopen = () => console.log("SSE EventSource stream opened");
    es.onerror = () => console.error("SSE stream error");

    es.onmessage = async (m) => {
      try {
        const ev = JSON.parse(m.data) as QEvent;
        const msgLow = ev.message.toLowerCase();

        // 1. Process GNB connection state transitions
        const isSuccess =
          msgLow.includes("ng setup successful") ||
          msgLow.includes("s1 setup successful") ||
          (ev.payload && (ev.payload as any).status === "connected") ||
          (ev.payload && (ev.payload as any).success === true && (msgLow.includes("setup request") || msgLow.includes("setup response")));
          
        const isFailure =
          msgLow.includes("rejected") ||
          msgLow.includes("mismatch") ||
          (ev.payload && (ev.payload as any).status === "failed") ||
          (ev.payload && (ev.payload as any).success === false);

        if (isSuccess) {
          const payload = ev.payload as any || {};
          const gnbName = String(payload.gnb_name || payload.enb_name || "Nokia AirScale");
          const negotiatedPlmn = String(payload.plmn || "001/01");
          const negotiatedTac = String(payload.tac || "1");
          const negotiatedSlice = String(payload.slice || "eMBB");

          set((state) => ({
            connection: {
              ...state.connection,
              state: "connected",
              gnbName,
              negotiatedPlmn,
              negotiatedTac,
              negotiatedSlice,
            },
          }));
        } else if (isFailure && msgLow.includes("setup")) {
          const payload = ev.payload as any || {};
          const failReason = String(payload.failure_cause || payload.reason || "NG Setup rejected");
          const sentPlmn = String(payload.sent_plmn || "310/260");
          const fixGnb = String(payload.fix_gnb_side || payload.fix_gnb || "Check configuration");
          const fixQCore = String(payload.fix_qcore_side || payload.fix_qcore || "Validate network settings");

          set((state) => ({
            connection: {
              ...state.connection,
              state: "failed",
              failReason,
              sentPlmn,
              fixGnb,
              fixQCore,
            },
          }));
        }

        // 2. Process Trace Stream
        set((state) => {
          const newEvents = [...state.traceState.events, ev].slice(-200);
          const activeJourney = ev.journey_id || state.traceState.journeyId;

          return {
            traceState: {
              ...state.traceState,
              events: newEvents,
              journeyId: activeJourney,
            },
          };
        });

        // 3. Trigger Real Diagnostics if there is an error
        const hasError = ev.severity === "error";
        const activeJourney = ev.journey_id || get().traceState.journeyId;
        if (hasError && activeJourney) {
          try {
            const diag = await api.diagnoseJourney(activeJourney);
            set((state) => ({
              traceState: {
                ...state.traceState,
                diagnostic: diag,
              },
            }));
          } catch (e) {
            console.error("Failed to fetch diagnostics", e);
          }
        }

      } catch (err) {
        // ignore malformed events
      }
    };
  },

  closeEventSource: () => {
    if (esInstance) {
      esInstance.close();
      esInstance = null;
    }
  },

  resetToLive: () => {
    // Re-evaluate config into connection
    const { config, mode } = get();
    if (config) {
      const fields = connectionFieldsForMode(config, mode);
      set((state) => ({
        connection: {
          ...state.connection,
          state: "waiting", // Reset to waiting on live reconnect
          ...fields,
        },
      }));
    }
    get().initEventSource();
  },

  clearTrace: () => {
    set((state) => ({
      traceState: {
        ...state.traceState,
        events: [],
        streaming: false,
        activeScenario: null,
        diagnostic: null,
        journeyId: null,
        error: null,
      },
    }));
  },

  startSimulator: async (scenario?: string) => {
    const mode = get().mode;
    const activeScenario = scenario || "happy_path";
    get().clearTrace();
    set((state) => ({
      traceState: {
        ...state.traceState,
        streaming: true,
        activeScenario,
        mode,
        events: [],
        diagnostic: null,
        journeyId: null,
        error: null,
      },
    }));

    try {
      if (scenario) {
        await api.simulatorInject(mode, scenario);
      } else {
        await api.simulatorStart(mode);
      }

      for (let attempt = 0; attempt < 25; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 800));
        const status = await api.simulatorStatus();
        if (status.state === "success" || status.state === "failed") {
          set((state) => ({
            traceState: {
              ...state.traceState,
              streaming: false,
              journeyId: status.last_journey || state.traceState.journeyId,
              error: status.last_error || null,
            },
          }));
          return;
        }
      }

      set((state) => ({
        traceState: {
          ...state.traceState,
          streaming: false,
          error: "Simulator did not finish within the dashboard polling window.",
        },
      }));
    } catch (e) {
      set((state) => ({
        traceState: {
          ...state.traceState,
          streaming: false,
          error: (e as Error).message,
        },
      }));
    }
  },

  runSavedScenario: async (name: string) => {
    get().clearTrace();
    const mode = get().mode;
    set((state) => ({
      traceState: {
        ...state.traceState,
        streaming: true,
        activeScenario: name,
        mode,
        events: [],
        diagnostic: null,
        journeyId: null,
        error: null,
      },
    }));

    try {
      const result = await api.runScenario(name);
      let diagnostic: DiagnosticResult | null = null;
      if (result.journey_id && result.actual.result === "failure") {
        try {
          diagnostic = await api.diagnoseJourney(result.journey_id);
        } catch (e) {
          console.error("Failed to fetch scenario diagnostics", e);
        }
      }
      set((state) => ({
        traceState: {
          ...state.traceState,
          streaming: false,
          journeyId: result.journey_id || state.traceState.journeyId,
          diagnostic: diagnostic || state.traceState.diagnostic,
          error: result.error || result.actual.error || null,
        },
      }));
      return result;
    } catch (e) {
      set((state) => ({
        traceState: {
          ...state.traceState,
          streaming: false,
          error: (e as Error).message,
        },
      }));
      throw e;
    }
  },

  setMode: (mode: ProtocolMode) => {
    set((state) => ({
      mode,
      traceState: { ...state.traceState, mode },
      connection: state.config
        ? {
            ...state.connection,
            state: "waiting",
            ...connectionFieldsForMode(state.config, mode),
          }
        : state.connection,
    }));
  },
}));
