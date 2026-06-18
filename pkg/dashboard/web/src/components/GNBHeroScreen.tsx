import { useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import {
  Activity,
  ArrowRight,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  Play,
  RefreshCw,
} from "lucide-react";
import { api } from "../api/client";
import type { CoreHealth, NFStatus, QEvent } from "../api/types";
import { useConnectionStore } from "../stores/connectionStore";

interface GNBHeroScreenProps {
  onRegisterUE: () => void;
  onStartTrace: () => void;
  onOpenDiagnosis: () => void;
}

const SCENARIOS = [
  { id: "wrong_ki", label: "Wrong Ki" },
  { id: "wrong_plmn", label: "Wrong PLMN" },
  { id: "unprovisioned_imsi", label: "Unknown IMSI" },
  { id: "wrong_mme_address", label: "Timeout" },
] as const;

const NF_DESCRIPTIONS: Record<string, string> = {
  amf: "Access & mobility",
  mme: "4G mobility",
  smf: "Session control",
  upf: "User plane",
  ausf: "Authentication",
  udm: "Subscriber data",
  udr: "Data repository",
  nrf: "Discovery",
  nssf: "Slice selection",
  pcf: "Policy control",
  spgw: "4G gateway",
  hss: "4G subscriber DB",
  collector: "Telemetry bus",
};

export default function GNBHeroScreen({
  onRegisterUE,
  onStartTrace,
  onOpenDiagnosis,
}: GNBHeroScreenProps) {
  const connection = useConnectionStore((s) => s.connection);
  const loading = useConnectionStore((s) => s.loading);
  const mode = useConnectionStore((s) => s.mode);
  const setMode = useConnectionStore((s) => s.setMode);
  const startSimulator = useConnectionStore((s) => s.startSimulator);
  const traceState = useConnectionStore((s) => s.traceState);
  const reducedMotion = useReducedMotion();

  const [copied, setCopied] = useState(false);
  const [startingScenario, setStartingScenario] = useState<string | null>(null);
  const [nfPanelOpen, setNFPanelOpen] = useState(false);
  const [health, setHealth] = useState<CoreHealth | null>(null);
  const [expandedEvent, setExpandedEvent] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const loadHealth = async () => {
      try {
        const next = await api.health();
        if (!cancelled) setHealth(next);
      } catch {
        if (!cancelled) setHealth(null);
      }
    };
    loadHealth();
    const timer = window.setInterval(loadHealth, 4000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  const launchSimulator = async (scenario?: string) => {
    const key = scenario || "happy_path";
    setStartingScenario(key);
    try {
      const run = startSimulator(scenario);
      onStartTrace();
      await run;
    } finally {
      setStartingScenario(null);
    }
  };

  const copyAddress = async () => {
    await navigator.clipboard.writeText(connection.amfAddress);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1800);
  };

  const visibleEvents = useMemo(() => traceState.events.slice(-8), [traceState.events]);
  const latestError = useMemo(
    () => [...traceState.events].reverse().find((event) => event.severity === "error"),
    [traceState.events],
  );
  const state = connection.state;
  const isConnected = state === "connected";
  const isFailed = state === "failed";
  const modeTitle = mode === "5g" ? "gNB" : "eNB";
  const setupName = mode === "5g" ? "NG Setup" : "S1 Setup";
  const endpointLabel = mode === "5g" ? "AMF NGAP" : "MME S1AP";
  const connectionName = connection.gnbName || (mode === "5g" ? "Nokia AirScale" : "srsRAN eNodeB");
  const traceVisible = isConnected || isFailed || traceState.streaming || visibleEvents.length > 0;
  const stateLabel =
    state === "connected" ? "connected" : state === "failed" ? "setup mismatch" : "waiting";

  if (loading) {
    return (
      <div className="qhero-loading">
        <RefreshCw className="h-8 w-8 animate-spin" />
        <p>Querying core configuration...</p>
      </div>
    );
  }

  return (
    <div className={`qhero-stage qhero-${state}`}>
      <header className="qhero-topbar">
        <button
          type="button"
          className="qhero-brand"
          onClick={() => setMode(mode === "5g" ? "4g" : "5g")}
          aria-label={`Switch to ${mode === "5g" ? "4G EPC" : "5G SA"}`}
          title={`Current mode: ${mode === "5g" ? "5G SA" : "4G EPC"}`}
        >
          <span className="qhero-dot" />
          <b>QCore</b>
          <span>core online</span>
          <span className="qhero-mode-pill">{mode === "5g" ? "5G SA" : "4G EPC"}</span>
        </button>

        <button
          type="button"
          className="qhero-nf-toggle"
          onClick={() => setNFPanelOpen((open) => !open)}
          aria-expanded={nfPanelOpen}
        >
          <span className="qhero-grid-icon" aria-hidden="true">
            <i />
            <i />
            <i />
            <i />
          </span>
          12 functions <ChevronDown className="h-3.5 w-3.5" />
        </button>
      </header>

      <main className="qhero-main" aria-live="polite">
        <p className="qhero-eyebrow">Is your {modeTitle} connected?</p>

        <motion.div
          className="qhero-latch-wrap"
          initial={false}
          animate={{ scale: isFailed && !reducedMotion ? [1, 1.015, 1] : 1 }}
          transition={{ duration: 0.6 }}
        >
          <LatchEmblem state={state} reducedMotion={Boolean(reducedMotion)} />
        </motion.div>

        <AnimatePresence mode="wait">
          <motion.section
            key={state}
            className={`qhero-copy${isFailed ? " is-failed" : ""}`}
            initial={reducedMotion ? false : { opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={reducedMotion ? undefined : { opacity: 0, y: -8 }}
            transition={{ duration: 0.28 }}
          >
            {isConnected ? (
              <>
                <h1>{modeTitle} connected · {connectionName}</h1>
                <p className="qhero-subline">
                  PLMN {connection.negotiatedPlmn || connection.configuredPlmn} · TAC{" "}
                  {connection.negotiatedTac || connection.configuredTac} ·{" "}
                  {connection.negotiatedSlice || "eMBB"}
                </p>
                <div className="qhero-actions">
                  <button type="button" className="qhero-btn qhero-btn-primary" onClick={onRegisterUE}>
                    Register a UE <ArrowRight className="h-4 w-4" />
                  </button>
                  <button type="button" className="qhero-btn qhero-btn-ghost" onClick={onStartTrace}>
                    Open live trace
                  </button>
                </div>
              </>
            ) : isFailed ? (
              <>
                <h1>{modeTitle} rejected · {shortCause(connection.failReason)}</h1>
                <p className="qhero-subline">
                  {connection.failReason ||
                    `${modeTitle} sent ${connection.sentPlmn || "unknown"} · QCore expects ${connection.configuredPlmn}`}
                </p>
                <div className="qhero-actions">
                  <button type="button" className="qhero-btn qhero-btn-primary" onClick={onOpenDiagnosis}>
                    Open full diagnosis
                  </button>
                  <button
                    type="button"
                    className="qhero-btn qhero-btn-ghost"
                    disabled={traceState.streaming || startingScenario !== null}
                    onClick={() => launchSimulator()}
                  >
                    Retry {setupName}
                  </button>
                </div>
              </>
            ) : (
              <>
                <h1>Waiting for {modeTitle}...</h1>
                <div className="qhero-subgrid" aria-label={`${modeTitle} target configuration`}>
                  <span>{endpointLabel}</span>
                  <b>{connection.amfAddress}</b>
                  <button type="button" onClick={copyAddress} aria-label={`Copy ${endpointLabel} address`}>
                    {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </button>
                  <span>PLMN</span>
                  <b>{connection.configuredPlmn}</b>
                  <i />
                  <span>TAC</span>
                  <b>{connection.configuredTac}</b>
                  <i />
                </div>
                <div className="qhero-actions">
                  <button
                    type="button"
                    className="qhero-btn qhero-btn-primary"
                    disabled={traceState.streaming || startingScenario !== null}
                    onClick={() => launchSimulator()}
                  >
                    {startingScenario === "happy_path" ? (
                      <RefreshCw className="h-4 w-4 animate-spin" />
                    ) : (
                      <Play className="h-4 w-4" />
                    )}
                    Start simulator
                  </button>
                  <button type="button" className="qhero-btn qhero-btn-ghost" onClick={onStartTrace}>
                    Open live trace
                  </button>
                </div>
              </>
            )}
          </motion.section>
        </AnimatePresence>

        <div className="qhero-state-chips" aria-label="Current connection state">
          {(["waiting", "connected", "failed"] as const).map((chip) => (
            <span key={chip} className={chip === state ? "active" : ""}>
              {chip === "failed" ? "failed" : chip}
            </span>
          ))}
        </div>

        <section className="qhero-scenarios" aria-label="Built-in simulator scenarios">
          <button
            type="button"
            className="qhero-scenario-main"
            disabled={traceState.streaming || startingScenario !== null}
            onClick={() => launchSimulator()}
          >
            {startingScenario === "happy_path" ? "Starting happy path..." : "Run happy path"}
          </button>
          {SCENARIOS.map((scenario) => (
            <button
              type="button"
              key={scenario.id}
              disabled={traceState.streaming || startingScenario !== null}
              onClick={() => launchSimulator(scenario.id)}
            >
              {startingScenario === scenario.id ? "Starting..." : scenario.label}
            </button>
          ))}
        </section>

        {traceVisible && (
          <section className="qhero-trace" aria-label="Live signaling trace">
            <div className="qhero-trace-head">
              <div>
                <span>Live signaling</span>
                <small>{traceState.streaming ? "streaming" : stateLabel}</small>
              </div>
              <button type="button" onClick={onStartTrace}>
                full trace <ChevronRight className="h-3.5 w-3.5" />
              </button>
            </div>

            <div className="qhero-trace-list">
              {visibleEvents.length === 0 ? (
                <div className="qhero-empty-trace">Waiting for the first live event from /api/events/stream.</div>
              ) : (
                visibleEvents.map((event) => {
                  const expanded = expandedEvent === event.id;
                  const failed = event.severity === "error";
                  return (
                    <div key={event.id} className={`qhero-trace-item ${failed ? "failed" : ""}`}>
                      <button
                        type="button"
                        className="qhero-trace-row"
                        onClick={() => setExpandedEvent(expanded ? null : event.id)}
                      >
                        <time>{formatEventTime(event.timestamp)}</time>
                        <span className={`qhero-proto proto-${protocolFor(event).toLowerCase()}`}>
                          {protocolFor(event)}
                        </span>
                        <span>{event.message}</span>
                        <span>{expanded ? "hide" : "details"}</span>
                      </button>
                      <AnimatePresence initial={false}>
                        {expanded && (
                          <motion.div
                            className="qhero-event-detail"
                            initial={reducedMotion ? false : { height: 0, opacity: 0 }}
                            animate={{ height: "auto", opacity: 1 }}
                            exit={reducedMotion ? undefined : { height: 0, opacity: 0 }}
                          >
                            <pre>{formatEventDetail(event)}</pre>
                            {failed && (
                              <button type="button" onClick={onOpenDiagnosis}>
                                Open diagnosis for this failure
                              </button>
                            )}
                          </motion.div>
                        )}
                      </AnimatePresence>
                    </div>
                  );
                })
              )}
            </div>

            <div className="qhero-trace-foot">
              <Activity className="h-3.5 w-3.5" />
              <span>
                {latestError
                  ? `Last failure: ${latestError.message}`
                  : traceState.journeyId
                    ? `Journey ${traceState.journeyId}`
                    : "SSE feed ready"}
              </span>
            </div>
          </section>
        )}

      </main>

      <NFPanel open={nfPanelOpen} health={health} onClose={() => setNFPanelOpen(false)} />
    </div>
  );
}

function LatchEmblem({
  state,
  reducedMotion,
}: {
  state: "waiting" | "connected" | "failed";
  reducedMotion: boolean;
}) {
  return (
    <svg className="qhero-latch" viewBox="0 0 240 240" role="img" aria-label={`RAN state ${state}`}>
      <circle className="ring-bg" cx="120" cy="120" r="92" />
      <circle className="glow" cx="120" cy="120" r="92" />
      <circle className="lock-ring" cx="120" cy="120" r="72" />
      <motion.path
        className="ring-half ring-left"
        d="M120 28 A92 92 0 0 0 120 212"
        initial={false}
        animate={{ rotate: state === "connected" ? 0 : state === "failed" ? -9 : -26 }}
        transition={reducedMotion ? { duration: 0 } : { duration: 0.6, ease: [0.2, 0.8, 0.2, 1] }}
        transformOrigin="120px 120px"
      />
      <motion.path
        className="ring-half ring-right"
        d="M120 28 A92 92 0 0 1 120 212"
        initial={false}
        animate={{ rotate: state === "connected" ? 0 : state === "failed" ? 9 : 26 }}
        transition={reducedMotion ? { duration: 0 } : { duration: 0.6, ease: [0.2, 0.8, 0.2, 1] }}
        transformOrigin="120px 120px"
      />
      <path className="seam" d="M120 31 L120 209" />
      <motion.path
        className="check"
        d="M86 122 L110 146 L156 94"
        initial={false}
        animate={{ pathLength: state === "connected" ? 1 : 0, opacity: state === "connected" ? 1 : 0 }}
        transition={reducedMotion ? { duration: 0 } : { delay: 0.18, duration: 0.38 }}
      />
      <motion.g
        className="xmark"
        initial={false}
        animate={{ opacity: state === "failed" ? 1 : 0, scale: state === "failed" ? 1 : 0.86 }}
        transition={reducedMotion ? { duration: 0 } : { duration: 0.24 }}
        transformOrigin="120px 120px"
      >
        <path d="M92 92 L148 148" />
        <path d="M148 92 L92 148" />
      </motion.g>
      <motion.circle
        className="pulse"
        cx="120"
        cy="120"
        r="96"
        initial={false}
        animate={state === "waiting" && !reducedMotion ? { opacity: [0.2, 0.05, 0.2], scale: [1, 1.07, 1] } : {}}
        transition={{ repeat: Infinity, duration: 2.4 }}
      />
    </svg>
  );
}

function NFPanel({
  open,
  health,
  onClose,
}: {
  open: boolean;
  health: CoreHealth | null;
  onClose: () => void;
}) {
  const nfs = useMemo(() => normalizeNFs(health?.nfs ?? []), [health]);

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.button
            type="button"
            className="qhero-panel-scrim"
            onClick={onClose}
            aria-label="Close network function panel"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
          />
          <motion.aside
            className="qhero-nf-panel"
            initial={{ x: 360, opacity: 0 }}
            animate={{ x: 0, opacity: 1 }}
            exit={{ x: 360, opacity: 0 }}
            transition={{ type: "spring", stiffness: 220, damping: 28 }}
            aria-label="QCore network functions"
          >
            <div>
              <p>The core</p>
              <h2>12 network functions</h2>
              <span>one conceptual core</span>
            </div>
            <div className="qhero-nf-grid">
              {nfs.map((nf) => (
                <div key={nf.name} className={nf.reachable ? "active" : "idle"}>
                  <b>{nf.name.toUpperCase()}</b>
                  <span>{NF_DESCRIPTIONS[nf.name.toLowerCase()] || nf.status || "Network function"}</span>
                </div>
              ))}
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
}

// A fail reason can be a full sentence (e.g. "Authentication failure — the UE Ki
// does not match the Ki provisioned for this IMSI in QCore"). The hero headline
// must stay short and scannable, so we surface only the leading clause here (the
// part before an em/en/hyphen dash or colon) and cap its length; the complete
// sentence rides on the sub-line below.
function shortCause(reason?: string): string {
  if (!reason) return "PLMN mismatch";
  const head = reason.split(/\s[—–-]\s|: /)[0].trim();
  return head.length > 46 ? `${head.slice(0, 44).trimEnd()}…` : head;
}

function normalizeNFs(nfs: NFStatus[]) {
  const byName = new Map(nfs.map((nf) => [nf.name.toLowerCase(), nf]));
  const canonical = ["amf", "smf", "upf", "ausf", "udm", "udr", "nrf", "mme", "spgw", "hss", "collector", "pcf"];
  return canonical.map((name) => byName.get(name) || {
    name,
    url: "",
    reachable: false,
    latency_ms: 0,
    status: "idle",
  });
}

function protocolFor(event: QEvent) {
  if (event.protocol) return event.protocol.toUpperCase();
  const text = `${event.nf} ${event.category} ${event.message}`.toLowerCase();
  if (text.includes("nas") || text.includes("registration") || text.includes("authentication")) return "NAS";
  if (text.includes("pfcp") || text.includes("n4")) return "PFCP";
  if (text.includes("gtp")) return "GTP";
  if (text.includes("s1")) return "S1AP";
  return "NGAP";
}

function formatEventTime(timestamp: string) {
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return "--:--";
  return date.toLocaleTimeString([], {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatEventDetail(event: QEvent) {
  const payload = event.payload ? JSON.stringify(event.payload, null, 2) : "{}";
  return `${event.nf.toUpperCase()} · ${event.category} · ${event.severity}\n${payload}`;
}
