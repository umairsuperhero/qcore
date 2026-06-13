import { useState, useEffect } from "react";
import HealthView from "./components/HealthView";
import SubscribersView from "./components/SubscribersView";
import LiveTraceView from "./components/LiveTraceView";
import GNBHeroScreen from "./components/GNBHeroScreen";
import { useConnectionStore } from "./stores/connectionStore";

type Tab = "ran" | "health" | "subscribers" | "trace";

const TABS: { id: Tab; label: string }[] = [
  { id: "ran", label: "gNB Connection" },
  { id: "health", label: "System Health" },
  { id: "subscribers", label: "Subscribers" },
  { id: "trace", label: "Live Trace" },
];

export default function App() {
  const connection = useConnectionStore((s) => s.connection);
  const traceState = useConnectionStore((s) => s.traceState);
  const fetchConfig = useConnectionStore((s) => s.fetchConfig);
  const initEventSource = useConnectionStore((s) => s.initEventSource);
  const closeEventSource = useConnectionStore((s) => s.closeEventSource);

  const [tab, setTab] = useState<Tab>("ran");

  useEffect(() => {
    fetchConfig();
    initEventSource();
    return () => closeEventSource();
  }, [fetchConfig, initEventSource, closeEventSource]);

  const isConnected = connection.state === "connected";
  const hasTraceActivity =
    traceState.streaming ||
    traceState.events.length > 0 ||
    traceState.activeScenario !== null ||
    traceState.journeyId !== null;
  const canShowTrace = isConnected || hasTraceActivity;

  // Keep post-connection tabs gated, but allow an active simulator run to
  // navigate straight to Live Trace before setup flips the connection state.
  useEffect(() => {
    if (!isConnected && tab !== "ran" && !(tab === "trace" && hasTraceActivity)) {
      setTab("ran");
    }
  }, [hasTraceActivity, isConnected, tab]);

  return (
    <div className="min-h-screen bg-darkbg-950 text-slate-100 flex flex-col font-sans">
      {/* Premium Header */}
      <header className="bg-darkbg-900 border-b border-darkbg-700/80 sticky top-0 z-40 backdrop-blur-md bg-opacity-80">
        <div className="max-w-6xl mx-auto px-6 py-4 flex items-center justify-between">
          <div className="flex items-baseline gap-3">
            <h1 className="text-2xl font-black tracking-tight text-white flex items-center gap-2">
              <span className="bg-emerald-600 text-white px-2 py-0.5 rounded font-black text-sm tracking-normal">Q</span>
              Core
            </h1>
            <span className="text-xs text-slate-400 font-medium tracking-wide hidden sm:inline">
              cellular dev &amp; test environment
            </span>
          </div>

          {/* Connection Status indicator */}
          <div className="flex items-center gap-2 text-xs font-mono">
            {isConnected ? (
              <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
                RAN CONNECTED
              </span>
            ) : connection.state === "failed" ? (
              <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-amber-500/10 text-amber-400 border border-amber-500/20">
                <span className="w-1.5 h-1.5 rounded-full bg-amber-400 animate-pulse" />
                SETUP MISMATCH
              </span>
            ) : (
              <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-slate-500/10 text-slate-400 border border-slate-700/30">
                <span className="w-1.5 h-1.5 rounded-full bg-slate-500 animate-ping" />
                WAITING FOR gNB
              </span>
            )}
          </div>
        </div>

        {/* Navigation Tabs - revealed using Progressive Disclosure */}
        <div className="max-w-6xl mx-auto px-6">
          <nav className="flex gap-2 -mb-px">
            {TABS.map((t) => {
              if (t.id === "trace" && !canShowTrace) return null;
              if (t.id !== "ran" && t.id !== "trace" && !isConnected) return null;

              return (
                <button
                  key={t.id}
                  className={`tab border-b-2 py-3 px-4 text-sm font-medium transition cursor-pointer ${
                    tab === t.id
                      ? "tab-active border-emerald-500 text-emerald-400"
                      : "border-transparent text-slate-400 hover:text-white"
                  }`}
                  onClick={() => setTab(t.id)}
                >
                  {t.label}
                </button>
              );
            })}
          </nav>
        </div>
      </header>

      {/* Main View Area */}
      <main className="flex-1 max-w-6xl w-full mx-auto px-6 py-8">
        {tab === "ran" && (
          <GNBHeroScreen 
            onRegisterUE={() => setTab("subscribers")} 
            onStartTrace={() => setTab("trace")}
          />
        )}
        {isConnected && (
          <>
            {tab === "health" && <HealthView onStartSim={() => setTab("trace")} />}
            {tab === "subscribers" && <SubscribersView />}
          </>
        )}
        {tab === "trace" && canShowTrace && <LiveTraceView />}
      </main>

      {/* Footer Info */}
      <footer className="bg-darkbg-950 border-t border-darkbg-850 py-6 text-center text-xs text-slate-500">
        <div className="max-w-6xl mx-auto px-6 flex flex-col sm:flex-row items-center justify-between gap-4">
          <p>© 2026 QCore Systems. All rights reserved.</p>
          <div className="flex gap-4">
            <span className="text-slate-600 font-mono">v0.8.0-dev</span>
          </div>
        </div>
      </footer>
    </div>
  );
}
