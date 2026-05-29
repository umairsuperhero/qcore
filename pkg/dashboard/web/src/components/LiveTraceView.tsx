import { useEffect, useRef, useState } from "react";
import { getLearningContent } from "../data/learning";
import { api } from "../api/client";
import type { QEvent, SimulatorStatus, DiagnosticResult } from "../api/types";

// Cap the on-screen event buffer. Old events stay queryable in the
// collector — this is just a UI guard against runaway DOM.
const MAX_EVENTS = 200;

export default function LiveTraceView() {
  const [events, setEvents] = useState<QEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const [sim, setSim] = useState<SimulatorStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [learningMode, setLearningMode] = useState(false);
  const [showCustom, setShowCustom] = useState(false);
  const [customYaml, setCustomYaml] = useState(`name: "Custom Failure"
mode: "4g"
overrides:
  ki: "00000000000000000000000000000000"`);
  const [diagnosing, setDiagnosing] = useState(false);
  const [diagnostic, setDiagnostic] = useState<DiagnosticResult | null>(null);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const es = new EventSource("/api/events/stream");
    esRef.current = es;
    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);
    es.onmessage = (m) => {
      try {
        const ev = JSON.parse(m.data) as QEvent;
        setEvents((prev) => [...prev.slice(-MAX_EVENTS + 1), ev]);
      } catch {
        /* malformed; drop */
      }
    };
    return () => es.close();
  }, []);

  useEffect(() => {
    const tick = async () => {
      try {
        setSim(await api.simulatorStatus());
      } catch {
        /* silent — surfaced via the err panel in Health */
      }
    };
    tick();
    const t = setInterval(tick, 2000);
    return () => clearInterval(t);
  }, []);

  const [mode, setMode] = useState<"4g" | "5g">("5g");

  const start = async () => {
    setBusy(true);
    try {
      await api.simulatorStart(mode);
    } finally {
      setBusy(false);
    }
  };
  const inject = async (scenario: string) => {
    setBusy(true);
    try {
      await api.simulatorInject(mode, scenario);
    } finally {
      setBusy(false);
    }
  };
  const injectCustom = async () => {
    setBusy(true);
    try {
      await api.simulatorCustom(customYaml);
      setShowCustom(false);
    } catch (e) {
      alert("Failed to inject custom scenario: " + e);
    } finally {
      setBusy(false);
    }
  };
  const clear = () => {
    setEvents([]);
    setDiagnostic(null);
  };

  const diagnose = async (journeyID: string) => {
    if (!journeyID || journeyID === "(no journey)") return;
    setDiagnosing(true);
    setDiagnostic(null);
    try {
      const res = await api.diagnoseJourney(journeyID);
      setDiagnostic(res);
    } catch (e) {
      console.error(e);
      setDiagnostic({
        Matched: false,
        Explanation: "Failed to reach diagnostic service.",
        RootCause: "API Error",
        Fix: "Check dashboard backend logs.",
      });
    } finally {
      setDiagnosing(false);
    }
  };

  // Group by journey for the diagnose-this affordance.
  const byJourney = groupByJourney(events);
  const lastJourney = byJourney[byJourney.length - 1];
  const lastJourneyHasError = lastJourney?.events.some((e) => e.severity === "error");

  return (
    <div className="space-y-6">
      <div className="card flex flex-wrap items-center gap-3">
        <span
          className={`inline-flex items-center gap-2 text-sm font-medium ${
            connected ? "text-emerald-700" : "text-slateblue-500"
          }`}
        >
          <span
            className={`inline-block w-2 h-2 rounded-full ${
              connected ? "bg-emerald-500" : "bg-slateblue-500"
            }`}
          />
          {connected ? "Streaming" : "Disconnected"}
        </span>

        <div className="grow" />

        <div className="flex gap-4 text-sm font-medium mr-4">
          <label className="flex items-center gap-1 cursor-pointer">
            <input
              type="radio"
              name="trace_mode"
              value="4g"
              checked={mode === "4g"}
              onChange={() => setMode("4g")}
            />
            4G
          </label>
          <label className="flex items-center gap-1 cursor-pointer">
            <input
              type="radio"
              name="trace_mode"
              value="5g"
              checked={mode === "5g"}
              onChange={() => setMode("5g")}
            />
            5G
          </label>
          <label className="flex items-center gap-1 cursor-pointer">
            <input
              type="checkbox"
              checked={learningMode}
              onChange={(e) => setLearningMode(e.target.checked)}
            />
            <span className="text-indigo-600">Learning Mode</span>
          </label>
        </div>

        <button className="btn-primary" disabled={busy} onClick={start}>
          Run attach
        </button>
        <div className="flex flex-wrap gap-2">
          <ScenarioButton onClick={() => inject("wrong_ki")} disabled={busy}>
            Inject: wrong Ki
          </ScenarioButton>
          <ScenarioButton onClick={() => inject("wrong_plmn")} disabled={busy}>
            Inject: wrong PLMN
          </ScenarioButton>
          <ScenarioButton onClick={() => inject("unprovisioned_imsi")} disabled={busy}>
            Inject: unprovisioned IMSI
          </ScenarioButton>
          <ScenarioButton onClick={() => inject("wrong_mme_address")} disabled={busy}>
            Inject: wrong MME address
          </ScenarioButton>
          <ScenarioButton onClick={() => setShowCustom(!showCustom)} disabled={busy}>
            Inject: Custom YAML...
          </ScenarioButton>
        </div>
        <button className="btn-secondary" onClick={clear}>
          Clear
        </button>
      </div>

      {showCustom && (
        <div className="card border-slateblue-200 bg-white space-y-3">
          <h3 className="text-sm font-semibold text-slateblue-800">Custom Scenario Definition</h3>
          <textarea
            className="w-full h-32 p-2 text-sm font-mono bg-slateblue-50 border border-slateblue-200 rounded"
            value={customYaml}
            onChange={(e) => setCustomYaml(e.target.value)}
          />
          <div className="flex gap-2">
            <button className="btn-primary" onClick={injectCustom} disabled={busy}>
              Run Custom Scenario
            </button>
            <button className="btn-secondary" onClick={() => setShowCustom(false)}>
              Cancel
            </button>
          </div>
        </div>
      )}

      {sim?.last_scenario && (
        <div className="text-sm text-slateblue-500">
          Last scenario: <span className="font-mono">{sim.last_scenario}</span>
          {sim.state === "failed" && sim.last_error && (
            <> — failed with: {sim.last_error}</>
          )}
        </div>
      )}

      {lastJourneyHasError && !diagnostic && (
        <div className="card border-amber-200 bg-amber-50 flex items-center justify-between">
          <div>
            <h2 className="text-amber-800">This attach ended in an error.</h2>
            <p className="text-sm text-amber-700 mt-1">
              Click to diagnose the root cause using the QCore Diagnostic AI.
            </p>
          </div>
          <button
            className="btn-primary"
            disabled={diagnosing || !lastJourney?.id || lastJourney.id === "(no journey)"}
            onClick={() => lastJourney?.id && diagnose(lastJourney.id)}
          >
            {diagnosing ? "Diagnosing..." : "Diagnose this"}
          </button>
        </div>
      )}

      {diagnostic && (
        <div className="card border-indigo-200 bg-indigo-50 space-y-4 animate-glow">
          <div className="flex items-center gap-2">
            <span className="text-2xl">✨</span>
            <h2 className="text-indigo-900 text-lg font-semibold">Diagnostic Report</h2>
          </div>
          <div className="space-y-3 text-indigo-900 text-sm">
            <div>
              <h3 className="font-semibold text-xs uppercase tracking-wider text-indigo-700">What Happened</h3>
              <p>{diagnostic.Explanation}</p>
            </div>
            <div>
              <h3 className="font-semibold text-xs uppercase tracking-wider text-indigo-700">Root Cause</h3>
              <p>{diagnostic.RootCause}</p>
            </div>
            <div className="bg-white/60 p-3 rounded border border-indigo-100">
              <h3 className="font-semibold text-xs uppercase tracking-wider text-indigo-700 mb-1">Suggested Fix</h3>
              <p>{diagnostic.Fix}</p>
            </div>
          </div>
        </div>
      )}

      <div className="space-y-2">
        {events.length === 0 ? (
          <div className="card text-slateblue-500 text-sm">
            No events yet. Click "Run attach" or inject a failure scenario
            above to produce a trace.
          </div>
        ) : (
          events.map((ev) => <EventRow key={ev.id} ev={ev} learningMode={learningMode} />)
        )}
      </div>
    </div>
  );
}

function ScenarioButton({
  children,
  onClick,
  disabled,
}: {
  children: React.ReactNode;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button className="btn-secondary text-xs" disabled={disabled} onClick={onClick}>
      {children}
    </button>
  );
}

function EventRow({ ev, learningMode }: { ev: QEvent; learningMode?: boolean }) {
  const [open, setOpen] = useState(false);
  const colors = severityColors(ev.severity);
  const learningHtml = learningMode ? getLearningContent(ev.message) : null;

  return (
    <div className={`card p-3 cursor-pointer animate-slide-in ${colors.border}`} onClick={() => setOpen((v) => !v)}>
      <div className="flex items-center gap-3">
        <span className={`text-[10px] uppercase tracking-wider font-semibold px-1.5 py-0.5 rounded ${colors.badge}`}>
          {ev.nf}
        </span>
        {ev.protocol && (
          <span className="text-[10px] uppercase tracking-wider text-slateblue-500">
            {ev.protocol}
          </span>
        )}
        <span className="text-sm text-slateblue-900 flex-1">{ev.message}</span>
        <span className="text-xs text-slateblue-500 font-mono">
          {timeOnly(ev.timestamp)}
        </span>
      </div>
      {ev.journey_id && (
        <div className="text-[10px] text-slateblue-500 font-mono mt-1">
          journey {ev.journey_id}
        </div>
      )}
      {open && ev.payload && (
        <pre className="mt-2 bg-slateblue-50 rounded p-2 text-xs overflow-x-auto font-mono">
          {JSON.stringify(ev.payload, null, 2)}
        </pre>
      )}
      {learningHtml && (
        <div className="mt-2 bg-indigo-50 border-l-4 border-indigo-400 p-2 text-xs text-indigo-900 rounded-r">
          <div dangerouslySetInnerHTML={{ __html: learningHtml.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>') }} />
        </div>
      )}
    </div>
  );
}

function severityColors(s: QEvent["severity"]) {
  switch (s) {
    case "error":
      return { border: "border-red-200 bg-red-50", badge: "bg-red-100 text-red-800" };
    case "warn":
      return { border: "border-amber-200 bg-amber-50", badge: "bg-amber-100 text-amber-800" };
    default:
      return { border: "", badge: "bg-slateblue-100 text-slateblue-700" };
  }
}

function timeOnly(ts: string): string {
  try {
    return new Date(ts).toLocaleTimeString([], { hour12: false });
  } catch {
    return ts;
  }
}

interface JourneyGroup {
  id: string;
  events: QEvent[];
}
function groupByJourney(events: QEvent[]): JourneyGroup[] {
  const groups: JourneyGroup[] = [];
  let current: JourneyGroup | null = null;
  for (const ev of events) {
    const id = ev.journey_id || "(no journey)";
    if (!current || current.id !== id) {
      current = { id, events: [] };
      groups.push(current);
    }
    current.events.push(ev);
  }
  return groups;
}
