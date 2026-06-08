import { create } from "zustand";
import { api } from "../api/client";
import type { QEvent, DiagnosticResult, RANConfig } from "../api/types";
import { getMockEvents, getMockDiagnostic } from "../api/traceStreamMock"; // We'll move mock logic here

export type GNBState = "waiting" | "connected" | "failed";

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
  mode: "4g" | "5g";
  diagnostic: DiagnosticResult | null;
  journeyId: string | null;
}

interface ConnectionStore {
  // Config & Status
  loading: boolean;
  config: RANConfig | null;
  connection: GNBConnectionData;

  // Trace Stream State
  traceState: TraceStreamState;

  // Simulation Overrides
  isSimulated: boolean;
  simulatedState: GNBState;
  isMockStream: boolean; // Replaces USE_MOCK_STREAM

  // Actions
  fetchConfig: () => Promise<void>;
  initEventSource: () => void;
  closeEventSource: () => void;
  triggerSimulation: (state: GNBState) => void;
  resetToLive: () => void;
  runScenario: (scenario: string, mode: "4g" | "5g") => void;
  clearTrace: () => void;
  setMode: (mode: "4g" | "5g") => void;
}

let esInstance: EventSource | null = null;
let timerRef: number | null = null;

const genId = () => Math.random().toString(36).substring(2, 9);

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

export const useConnectionStore = create<ConnectionStore>((set, get) => ({
  loading: true,
  config: null,
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
  },

  isSimulated: false,
  simulatedState: "waiting",
  isMockStream: false,

  fetchConfig: async () => {
    try {
      const cfg = await api.ranConfig();
      const amfAddr = endpointAddress(cfg.amf_address || cfg.mme_address, cfg.amf_ngap_port, 38412);
      const formattedPlmn = formatPlmn(cfg.amf_plmn || cfg.plmn);

      set((state) => {
        if (state.isSimulated) return { config: cfg, loading: false };
        return {
          config: cfg,
          loading: false,
          connection: {
            ...state.connection,
            amfAddress: amfAddr,
            configuredPlmn: formattedPlmn,
            configuredTac: String(cfg.amf_tac || cfg.tac),
          },
        };
      });
    } catch (err) {
      console.error("Failed to load RAN config:", err);
      set({ loading: false });
    }
  },

  initEventSource: () => {
    if (get().isSimulated) return;
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
          if (state.isMockStream) return state; // Ignore real events if mock stream is active
          
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

  triggerSimulation: (state: GNBState) => {
    get().closeEventSource();
    set({ isSimulated: true, simulatedState: state });
    
    // Update connection with simulated values
    const { config } = get();
    const defaultAmf = config
      ? endpointAddress(config.amf_address || config.mme_address, config.amf_ngap_port, 38412)
      : "192.168.1.50:38412";
    
    const defaultPlmn = config ? config.amf_plmn || config.plmn : "00101";
    const formattedPlmn = formatPlmn(defaultPlmn);
    const defaultTac = config ? String(config.amf_tac || config.tac) : "1";

    if (state === "waiting") {
      set((s) => ({
        connection: { ...s.connection, state: "waiting", amfAddress: defaultAmf, configuredPlmn: formattedPlmn, configuredTac: defaultTac },
      }));
    } else if (state === "connected") {
      set((s) => ({
        connection: {
          ...s.connection,
          state: "connected",
          amfAddress: defaultAmf,
          configuredPlmn: formattedPlmn,
          configuredTac: defaultTac,
          gnbName: "Nokia AirScale",
          negotiatedPlmn: "001/01",
          negotiatedTac: "1",
          negotiatedSlice: "eMBB",
        },
      }));
    } else if (state === "failed") {
      set((s) => ({
        connection: {
          ...s.connection,
          state: "failed",
          amfAddress: defaultAmf,
          configuredPlmn: formattedPlmn,
          configuredTac: defaultTac,
          failReason: "NG Setup rejected",
          sentPlmn: "310/260",
          fixGnb: "set MCC=001, MNC=01",
          fixQCore: "Change QCore to 310/260",
        },
      }));
    }
  },

  resetToLive: () => {
    set({ isSimulated: false, isMockStream: false });
    // Re-evaluate config into connection
    const { config } = get();
    if (config) {
      const amfAddr = endpointAddress(config.amf_address || config.mme_address, config.amf_ngap_port, 38412);
      const formattedPlmn = formatPlmn(config.amf_plmn || config.plmn);
      set((state) => ({
        connection: {
          ...state.connection,
          state: "waiting", // Reset to waiting on live reconnect
          amfAddress: amfAddr,
          configuredPlmn: formattedPlmn,
          configuredTac: String(config.amf_tac || config.tac),
        },
      }));
    }
    get().initEventSource();
  },

  clearTrace: () => {
    if (timerRef) {
      window.clearTimeout(timerRef);
      timerRef = null;
    }
    set((state) => ({
      traceState: {
        ...state.traceState,
        events: [],
        streaming: false,
        activeScenario: null,
        diagnostic: null,
        journeyId: null,
      },
    }));
  },

  runScenario: (scenario: string, mode: "4g" | "5g") => {
    get().clearTrace();
    set({ isMockStream: true }); // Start mock scenario

    const jId = "jrn-" + genId();
    const mockEventsList = getMockEvents(scenario, mode, jId);

    set((state) => ({
      traceState: {
        ...state.traceState,
        streaming: true,
        activeScenario: scenario,
        mode,
        journeyId: jId,
        events: [],
      },
    }));

    let index = 0;
    const pushNextEvent = () => {
      if (index >= mockEventsList.length) {
        const diag = getMockDiagnostic(scenario);
        set((state) => ({
          traceState: {
            ...state.traceState,
            streaming: false,
            diagnostic: diag,
          },
        }));
        return;
      }

      const nextEv = mockEventsList[index];
      set((state) => ({
        traceState: {
          ...state.traceState,
          events: [...state.traceState.events, nextEv],
        },
      }));

      index++;
      const delay = 400 + Math.random() * 400;
      timerRef = window.setTimeout(pushNextEvent, delay);
    };

    timerRef = window.setTimeout(pushNextEvent, 200);
  },

  setMode: (mode: "4g" | "5g") => {
    set((state) => ({
      traceState: { ...state.traceState, mode },
    }));
  },
}));
