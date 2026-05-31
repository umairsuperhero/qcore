import { useState, useEffect, useRef } from "react";
import { api } from "./client";
import type { RANConfig, QEvent } from "./types";

export type GNBState = "waiting" | "connected" | "failed";

export interface GNBConnectionData {
  state: GNBState;
  amfAddress: string;
  configuredPlmn: string;
  configuredTac: string;
  // Connected state details
  gnbName?: string;
  negotiatedPlmn?: string;
  negotiatedTac?: string;
  negotiatedSlice?: string;
  // Failed state details
  failReason?: string;
  sentPlmn?: string;
  fixGnb?: string;
  fixQCore?: string;
}

// Custom hook to consume gNB Connection State
export function useGNBConnection() {
  const [loading, setLoading] = useState(true);
  const [config, setConfig] = useState<RANConfig | null>(null);
  const [connection, setConnection] = useState<GNBConnectionData>({
    state: "waiting",
    amfAddress: "Loading...",
    configuredPlmn: "...",
    configuredTac: "...",
  });

  // Simulator controls for development / manual override
  const [isSimulated, setIsSimulated] = useState(false);
  const [simulatedState, setSimulatedState] = useState<GNBState>("waiting");
  
  const esRef = useRef<EventSource | null>(null);

  // Fetch initial config
  useEffect(() => {
    let active = true;
    api.ranConfig()
      .then((cfg) => {
        if (!active) return;
        setConfig(cfg);
        setLoading(false);
      })
      .catch((err) => {
        console.error("Failed to load RAN config:", err);
        if (!active) return;
        setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  // Update connection data when config is fetched
  useEffect(() => {
    if (!config || isSimulated) return;

    // AMF/MME details
    const host = config.mme_address.startsWith("<") || config.mme_address === "0.0.0.0"
      ? window.location.hostname || "192.168.1.50"
      : config.mme_address;
    
    // We use the AMF 38412 port for display as standard in the design spec, or fallback to config
    const port = config.mme_s1ap_port || 38412;
    const amfAddr = `${host}:${port}`;
    
    // Format PLMN (e.g. 00101 -> 001 / 01)
    let formattedPlmn = config.plmn;
    if (config.plmn.length === 5) {
      formattedPlmn = `${config.plmn.slice(0, 3)} / ${config.plmn.slice(3)}`;
    } else if (config.plmn.length === 6) {
      formattedPlmn = `${config.plmn.slice(0, 3)} / ${config.plmn.slice(3)}`;
    }

    setConnection((prev) => ({
      ...prev,
      amfAddress: amfAddr,
      configuredPlmn: formattedPlmn,
      configuredTac: String(config.tac),
    }));
  }, [config, isSimulated]);

  // Real-time EventSource listener
  useEffect(() => {
    if (isSimulated) {
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
      return;
    }

    const es = new EventSource("/api/events/stream");
    esRef.current = es;

    es.onmessage = (m) => {
      try {
        const ev = JSON.parse(m.data) as QEvent;
        
        // Let's analyze events related to gNB connection / NG Setup.
        // The backend schemas are mapped below:
        
        // 1. Success transition
        // Real-world pattern: event with message containing "NG Setup successful" or payload indicating setup success.
        const isSuccess = 
          ev.message.toLowerCase().includes("ng setup successful") || 
          ev.message.toLowerCase().includes("s1 setup successful") ||
          (ev.payload && ev.payload.status === "connected");
        
        // 2. Reject/Failure transition
        // Real-world pattern: event with message containing "rejected" or payload indicating status failed.
        const isFailure = 
          ev.message.toLowerCase().includes("rejected") || 
          ev.message.toLowerCase().includes("mismatch") ||
          (ev.payload && ev.payload.status === "failed");

        if (isSuccess) {
          const payload = ev.payload || {};
          const gnbName = String(payload.gnb_name || payload.enb_name || "Nokia AirScale");
          const negotiatedPlmn = String(payload.plmn || "001/01");
          const negotiatedTac = String(payload.tac || "1");
          const negotiatedSlice = String(payload.slice || "eMBB");

          setConnection((prev) => ({
            ...prev,
            state: "connected",
            gnbName,
            negotiatedPlmn,
            negotiatedTac,
            negotiatedSlice,
          }));
        } else if (isFailure) {
          const payload = ev.payload || {};
          const failReason = String(payload.reason || "PLMN mismatch");
          const sentPlmn = String(payload.sent_plmn || "310/260");
          const fixGnb = String(payload.fix_gnb || "set MCC=001, MNC=01");
          const fixQCore = String(payload.fix_qcore || "Change QCore to 310/260");

          setConnection((prev) => ({
            ...prev,
            state: "failed",
            failReason,
            sentPlmn,
            fixGnb,
            fixQCore,
          }));
        }
      } catch (err) {
        // ignore malformed events
      }
    };

    return () => {
      if (es) es.close();
    };
  }, [isSimulated]);

  // Handle Simulated transitions manually for demonstration and evaluation
  useEffect(() => {
    if (!isSimulated) return;

    // Use default simulated details when forced
    const defaultAmf = config 
      ? `${config.mme_address.startsWith("<") || config.mme_address === "0.0.0.0" ? "192.168.1.50" : config.mme_address}:${config.mme_s1ap_port || 38412}`
      : "192.168.1.50:38412";
    const defaultPlmn = config ? config.plmn : "00101";
    let formattedPlmn = defaultPlmn;
    if (defaultPlmn.length >= 5) {
      formattedPlmn = `${defaultPlmn.slice(0, 3)} / ${defaultPlmn.slice(3)}`;
    }

    if (simulatedState === "waiting") {
      setConnection({
        state: "waiting",
        amfAddress: defaultAmf,
        configuredPlmn: formattedPlmn,
        configuredTac: config ? String(config.tac) : "1",
      });
    } else if (simulatedState === "connected") {
      setConnection({
        state: "connected",
        amfAddress: defaultAmf,
        configuredPlmn: formattedPlmn,
        configuredTac: config ? String(config.tac) : "1",
        gnbName: "Nokia AirScale",
        negotiatedPlmn: "001/01",
        negotiatedTac: "1",
        negotiatedSlice: "eMBB",
      });
    } else if (simulatedState === "failed") {
      setConnection({
        state: "failed",
        amfAddress: defaultAmf,
        configuredPlmn: formattedPlmn,
        configuredTac: config ? String(config.tac) : "1",
        failReason: "NG Setup rejected",
        sentPlmn: "310/260",
        fixGnb: "set MCC=001, MNC=01",
        fixQCore: "Change QCore to 310/260",
      });
    }
  }, [isSimulated, simulatedState, config]);

  // Function to simulate a state from the developer panel
  const triggerSimulation = (state: GNBState) => {
    setIsSimulated(true);
    setSimulatedState(state);
  };

  // Reset to live mode
  const resetToLive = () => {
    setIsSimulated(false);
  };

  return {
    connection,
    loading,
    isSimulated,
    simulatedState,
    triggerSimulation,
    resetToLive,
  };
}
