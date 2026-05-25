import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { QEvent, SimulatorStatus } from "../api/types";

// Cap the on-screen event buffer. Old events stay queryable in the
// collector — this is just a UI guard against runaway DOM.
const MAX_EVENTS = 200;

export default function LiveTraceView() {
  const [events, setEvents] = useState<QEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const [sim, setSim] = useState<SimulatorStatus | null>(null);
  const [busy, setBusy] = useState(false);
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

  const start = async () => {
    setBusy(true);
    try {
      await api.simulatorStart();
    } finally {
      setBusy(false);
    }
  };
  const inject = async (scenario: string) => {
    setBusy(true);
    try {
      await api.simulatorInject(scenario);
    } finally {
      setBusy(false);
    }
  };
  const clear = () => setEvents([]);

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
        </div>
        <button className="btn-secondary" onClick={clear}>
          Clear
        </button>
      </div>

      {sim?.last_scenario && (
        <div className="text-sm text-slateblue-500">
          Last scenario: <span className="font-mono">{sim.last_scenario}</span>
          {sim.state === "failed" && sim.last_error && (
            <> — failed with: {sim.last_error}</>
          )}
        </div>
      )}

      {lastJourneyHasError && (
        <div className="card border-amber-200 bg-amber-50 flex items-center justify-between">
          <div>
            <h2 className="text-amber-800">This attach ended in an error.</h2>
            <p className="text-sm text-amber-700 mt-1">
              The diagnostic AI will explain the cause and the fix in plain
              language. Arriving in Phase C.
            </p>
          </div>
          <button className="btn-primary opacity-60 cursor-not-allowed" disabled title="Phase C — diagnostic AI not yet wired">
            Diagnose this
          </button>
        </div>
      )}

      <div className="space-y-2">
        {events.length === 0 ? (
          <div className="card text-slateblue-500 text-sm">
            No events yet. Click "Run attach" or inject a failure scenario
            above to produce a trace.
          </div>
        ) : (
          events.map((ev) => <EventRow key={ev.id} ev={ev} />)
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

function EventRow({ ev }: { ev: QEvent }) {
  const [open, setOpen] = useState(false);
  const colors = severityColors(ev.severity);
  return (
    <div className={`card p-3 cursor-pointer ${colors.border}`} onClick={() => setOpen((v) => !v)}>
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
