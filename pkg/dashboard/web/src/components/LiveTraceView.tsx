import { useState } from "react";
import { useConnectionStore } from "../stores/connectionStore";
import JourneyTimeline from "./JourneyTimeline";
import { getLearningContent } from "../data/learning";
import type { QEvent } from "../api/types";
import { 
  Play, 
  Trash2, 
  HelpCircle, 
  Activity, 
  Sliders, 
  BookOpen
} from "lucide-react";

export default function LiveTraceView() {
  const traceState = useConnectionStore((s) => s.traceState);
  const isMockStream = useConnectionStore((s) => s.isMockStream);
  const runScenario = useConnectionStore((s) => s.runScenario);
  const clearTrace = useConnectionStore((s) => s.clearTrace);
  const setMode = useConnectionStore((s) => s.setMode);

  const { events, streaming, activeScenario, mode, diagnostic } = traceState;
  const isMock = isMockStream;
  const clear = clearTrace;

  const [learningMode, setLearningMode] = useState(false);
  const [activeView, setActiveView] = useState<"timeline" | "raw">("timeline");

  // Handler for running standard attach
  const handleRunAttach = () => {
    runScenario("happy_path", mode);
  };

  // Handler for failure injection
  const handleInjectFailure = (scenario: string) => {
    runScenario(scenario, mode);
  };

  return (
    <div className="space-y-6">
      {/* Simulation & Stream Header Control Card */}
      <div className="card border-darkbg-700 bg-darkbg-900 flex flex-col gap-4 p-6 relative overflow-hidden">
        {/* Glow accent */}
        <div className="absolute top-0 right-0 w-36 h-36 bg-emerald-500/5 rounded-full blur-2xl pointer-events-none" />

        <div className="flex flex-wrap items-center justify-between gap-4 border-b border-darkbg-800 pb-4">
          <div className="flex items-center gap-3">
            <div className={`p-2 rounded-lg ${streaming ? "bg-emerald-500/10 text-emerald-400" : "bg-slate-800 text-slate-400"}`}>
              <Activity className={`w-5 h-5 ${streaming ? "animate-pulse" : ""}`} />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white tracking-tight">
                Live Signaling Monitor
              </h2>
              <p className="text-xs text-slate-400 font-medium">
                {isMock ? "Mock verification environment active" : "Real-time SSE event stream connected"}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-3">
            {/* View Toggle tabs */}
            <div className="bg-darkbg-950 p-1 rounded-lg border border-darkbg-800 flex">
              <button
                className={`text-xs px-3 py-1.5 rounded-md font-semibold transition ${
                  activeView === "timeline"
                    ? "bg-darkbg-900 text-emerald-400 shadow-sm border border-darkbg-700"
                    : "text-slate-400 hover:text-white"
                }`}
                onClick={() => setActiveView("timeline")}
              >
                Journey Timeline
              </button>
              <button
                className={`text-xs px-3 py-1.5 rounded-md font-semibold transition ${
                  activeView === "raw"
                    ? "bg-darkbg-900 text-emerald-400 shadow-sm border border-darkbg-700"
                    : "text-slate-400 hover:text-white"
                }`}
                onClick={() => setActiveView("raw")}
              >
                Raw Logs ({events.length})
              </button>
            </div>

            {/* Clear Button */}
            <button 
              className="btn-secondary text-xs flex items-center gap-1.5 hover:text-red-400 hover:border-red-950/40"
              onClick={clear}
            >
              <Trash2 className="w-3.5 h-3.5" />
              Clear
            </button>
          </div>
        </div>

        {/* Action Panel: Simulator Trigger Options */}
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pt-2">
          {/* Mode selections & Learning check */}
          <div className="flex flex-wrap items-center gap-5 text-sm font-semibold">
            <div className="flex items-center gap-3 bg-darkbg-950 px-3 py-1.5 rounded-lg border border-darkbg-800">
              <span className="text-xs text-slate-400 uppercase tracking-widest">Core Mode:</span>
              <label className="flex items-center gap-1.5 cursor-pointer text-slate-300 hover:text-white">
                <input
                  type="radio"
                  name="trace_mode"
                  value="4g"
                  checked={mode === "4g"}
                  onChange={() => setMode("4g")}
                  className="accent-emerald-500"
                />
                4G (EPC)
              </label>
              <label className="flex items-center gap-1.5 cursor-pointer text-slate-300 hover:text-white">
                <input
                  type="radio"
                  name="trace_mode"
                  value="5g"
                  checked={mode === "5g"}
                  onChange={() => setMode("5g")}
                  className="accent-emerald-500"
                />
                5G (SA)
              </label>
            </div>

            <label className="flex items-center gap-2 cursor-pointer text-slate-300 hover:text-white">
              <input
                type="checkbox"
                checked={learningMode}
                onChange={(e) => setLearningMode(e.target.checked)}
                className="rounded accent-emerald-500 bg-darkbg-950 border-darkbg-800"
              />
              <BookOpen className="w-4 h-4 text-emerald-500" />
              <span className="text-xs font-semibold uppercase tracking-wider">Learning Mode</span>
            </label>
          </div>

          {/* Trigger controls */}
          <div className="flex flex-wrap items-center gap-2">
            <button 
              className="btn-primary text-xs flex items-center gap-1.5 font-bold" 
              disabled={streaming} 
              onClick={handleRunAttach}
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              Run Happy Path
            </button>

            <div className="h-6 w-px bg-darkbg-800 hidden sm:block" />

            <div className="flex flex-wrap gap-1.5">
              <button 
                className="btn-secondary text-[11px] font-medium border-amber-950/20 text-amber-400 bg-amber-950/5 hover:bg-amber-950/20"
                disabled={streaming} 
                onClick={() => handleInjectFailure("wrong_ki")}
              >
                Fail: Wrong Ki
              </button>
              <button 
                className="btn-secondary text-[11px] font-medium border-amber-950/20 text-amber-400 bg-amber-950/5 hover:bg-amber-950/20"
                disabled={streaming} 
                onClick={() => handleInjectFailure("unprovisioned_imsi")}
              >
                Fail: Bad IMSI
              </button>
              <button 
                className="btn-secondary text-[11px] font-medium border-amber-950/20 text-amber-400 bg-amber-950/5 hover:bg-amber-950/20"
                disabled={streaming} 
                onClick={() => handleInjectFailure("wrong_mme_address")}
              >
                Fail: Timeout
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Active scenario metadata banner */}
      {activeScenario && (
        <div className="text-xs font-mono text-slate-400 flex items-center gap-2 bg-darkbg-900/40 p-3 rounded-lg border border-darkbg-850">
          <Sliders className="w-4 h-4 text-emerald-500" />
          <span>Active scenario: <strong className="text-white font-bold">{activeScenario.toUpperCase()}</strong></span>
          {streaming && <span className="animate-pulse text-emerald-400"> (Streaming events...)</span>}
        </div>
      )}

      {/* Main content display based on active toggle view */}
      {activeView === "timeline" ? (
        events.length === 0 ? (
          <div className="card text-slate-400 text-sm text-center py-16 border-dashed border-darkbg-700 bg-darkbg-950/20">
            <HelpCircle className="w-10 h-10 mx-auto text-slate-600 mb-3" />
            <p className="font-semibold text-slate-300">Journey stream is empty</p>
            <p className="text-xs text-slate-500 mt-1 max-w-sm mx-auto leading-relaxed">
              Click "Run Happy Path" or trigger a failure scenario to watch the UE signaling steps stream in live.
            </p>
          </div>
        ) : (
          <JourneyTimeline 
            events={events} 
            streaming={streaming} 
            diagnostic={diagnostic} 
          />
        )
      ) : (
        /* FALLBACK Chronological Raw Logs view (Progressive Disclosure) */
        <div className="space-y-2.5">
          {events.length === 0 ? (
            <div className="card text-slate-500 text-sm text-center py-12 border-dashed border-darkbg-700 bg-darkbg-950/20">
              No raw frames captured.
            </div>
          ) : (
            events.map((ev) => (
              <EventRow key={ev.id} ev={ev} learningMode={learningMode} />
            ))
          )}
        </div>
      )}
    </div>
  );
}

// Subcomponent: Raw Chronological Event Frame Row
function EventRow({ ev, learningMode }: { ev: QEvent; learningMode?: boolean }) {
  const [open, setOpen] = useState(false);
  const colors = severityColors(ev.severity);
  const learningHtml = learningMode ? getLearningContent(ev.message) : null;

  return (
    <div 
      className={`card p-4 cursor-pointer hover:border-slate-700 transition duration-150 ${colors.border}`} 
      onClick={() => setOpen((v) => !v)}
    >
      <div className="flex flex-wrap items-center gap-3">
        <span className={`text-[10px] uppercase tracking-wider font-bold px-2 py-0.5 rounded border ${colors.badge}`}>
          {ev.nf}
        </span>
        {ev.protocol && (
          <span className="text-[10px] uppercase tracking-wider text-slate-400 font-semibold">
            {ev.protocol}
          </span>
        )}
        <span className="text-sm font-semibold text-slate-200 flex-1 leading-normal">
          {ev.message}
        </span>
        <span className="text-xs text-slate-500 font-mono">
          {new Date(ev.timestamp).toLocaleTimeString([], { hour12: false })}
        </span>
      </div>

      {ev.journey_id && (
        <div className="text-[10px] text-slate-500 font-mono mt-1.5 pl-1.5 border-l border-slate-800">
          Journey correlation ID: {ev.journey_id}
        </div>
      )}

      {open && ev.payload && (
        <pre className="mt-3 bg-darkbg-950/80 rounded-xl p-4 text-xs overflow-x-auto font-mono text-slate-350 border border-darkbg-800 leading-relaxed max-h-[300px]">
          {JSON.stringify(ev.payload, null, 2)}
        </pre>
      )}

      {learningHtml && (
        <div className="mt-3 bg-indigo-950/20 border-l-4 border-indigo-500 p-3 text-xs text-indigo-300 rounded-r">
          <div dangerouslySetInnerHTML={{ __html: learningHtml.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>') }} />
        </div>
      )}
    </div>
  );
}

function severityColors(s: QEvent["severity"]) {
  switch (s) {
    case "error":
      return { 
        border: "border-red-500/25 bg-red-950/5", 
        badge: "bg-red-500/10 text-red-400 border-red-500/20" 
      };
    case "warn":
      return { 
        border: "border-amber-500/25 bg-amber-950/5", 
        badge: "bg-amber-500/10 text-amber-400 border-amber-500/20" 
      };
    default:
      return { 
        border: "border-darkbg-700/60 bg-darkbg-900/30", 
        badge: "bg-slate-800 text-slate-300 border-slate-700/50" 
      };
  }
}
