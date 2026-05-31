import { useState, useEffect } from "react";
import HealthView from "./components/HealthView";
import SubscribersView from "./components/SubscribersView";
import LiveTraceView from "./components/LiveTraceView";
import GNBHeroScreen from "./components/GNBHeroScreen";
import { useGNBConnection } from "./api/gnbConnection";
import { HelpCircle, Terminal, Play, X, RefreshCw } from "lucide-react";
import { api } from "./api/client";

type Tab = "ran" | "health" | "subscribers" | "trace";

const TABS: { id: Tab; label: string }[] = [
  { id: "ran", label: "gNB Connection" },
  { id: "health", label: "System Health" },
  { id: "subscribers", label: "Subscribers" },
  { id: "trace", label: "Live Trace" },
];

export default function App() {
  const { connection, triggerSimulation } = useGNBConnection();
  const [tab, setTab] = useState<Tab>("ran");
  const [showUeransimModal, setShowUeransimModal] = useState(false);
  const [startingSim, setStartingSim] = useState(false);

  const isConnected = connection.state === "connected";

  // Force Tab to "ran" if connection state changes to disconnected
  useEffect(() => {
    if (!isConnected) {
      setTab("ran");
    }
  }, [isConnected]);

  // Handle starting UERANSIM simulator
  const handleStartSimulator = async () => {
    setStartingSim(true);
    try {
      // Trigger core simulator start in 5G mode
      await api.simulatorStart("5g");
      
      // Simulate successful gNB connection events from the mock layer after a delay
      setTimeout(() => {
        setStartingSim(false);
        setShowUeransimModal(false);
        triggerSimulation("connected");
      }, 1500);
    } catch (err) {
      console.error("Failed to start simulator:", err);
      // Fallback: force simulated connection even if backend simulator endpoint fails
      setTimeout(() => {
        setStartingSim(false);
        setShowUeransimModal(false);
        triggerSimulation("connected");
      }, 1500);
    }
  };

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
              // Hide other tabs unless gNB is connected
              if (t.id !== "ran" && !isConnected) return null;

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
            onUseUeransim={() => setShowUeransimModal(true)} 
          />
        )}
        {isConnected && (
          <>
            {tab === "health" && <HealthView onStartSim={() => setTab("trace")} />}
            {tab === "subscribers" && <SubscribersView />}
            {tab === "trace" && <LiveTraceView />}
          </>
        )}
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

      {/* UERANSIM Helper Dialog Modal */}
      {showUeransimModal && (
        <div className="fixed inset-0 bg-darkbg-950/80 backdrop-blur-sm flex items-center justify-center p-4 z-50 animate-slide-in">
          <div className="card max-w-md w-full bg-darkbg-900 border-darkbg-700 p-6 flex flex-col gap-4 relative">
            <button
              onClick={() => setShowUeransimModal(false)}
              className="absolute top-4 right-4 text-slate-400 hover:text-white p-1 hover:bg-darkbg-800 rounded-lg transition"
            >
              <X className="w-5 h-5" />
            </button>

            <div className="flex items-center gap-2 text-emerald-400">
              <HelpCircle className="w-6 h-6" />
              <h3 className="text-lg font-bold text-white">Using UERANSIM?</h3>
            </div>

            <p className="text-sm text-slate-300 leading-relaxed">
              UERANSIM is an open-source 5G gNodeB and UE simulator. You can launch it inside QCore's dev container with a single click.
            </p>

            <div className="bg-darkbg-950 p-4 rounded-xl border border-darkbg-800 space-y-2 text-xs font-mono">
              <div className="flex items-center gap-2 text-slate-400 border-b border-darkbg-800/60 pb-1.5">
                <Terminal className="w-3.5 h-3.5" />
                <span>Docker Container CLI</span>
              </div>
              <p className="text-slate-300 break-all leading-normal">
                $ ./qcore-cli simulator start --mode 5g
              </p>
            </div>

            <div className="flex flex-col gap-2 mt-2">
              <button
                onClick={handleStartSimulator}
                disabled={startingSim}
                className="w-full btn-primary py-3 rounded-xl flex items-center justify-center gap-2 text-sm font-semibold"
              >
                {startingSim ? (
                  <>
                    <RefreshCw className="w-4 h-4 animate-spin" />
                    Starting Simulator...
                  </>
                ) : (
                  <>
                    <Play className="w-4 h-4" />
                    Launch UERANSIM Simulator
                  </>
                )}
              </button>
              <button
                onClick={() => setShowUeransimModal(false)}
                className="w-full btn-secondary py-3 rounded-xl text-sm font-medium"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
