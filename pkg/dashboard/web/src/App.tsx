import { useState } from "react";
import HealthView from "./components/HealthView";
import SubscribersView from "./components/SubscribersView";
import LiveTraceView from "./components/LiveTraceView";
import RANConnectView from "./components/RANConnectView";

type Tab = "health" | "subscribers" | "trace" | "ran";

const TABS: { id: Tab; label: string }[] = [
  { id: "health", label: "Health" },
  { id: "subscribers", label: "Subscribers" },
  { id: "trace", label: "Live trace" },
  { id: "ran", label: "Connect a real RAN" },
];

export default function App() {
  const [tab, setTab] = useState<Tab>("health");

  return (
    <div className="min-h-screen">
      <header className="bg-white border-b border-slateblue-100">
        <div className="max-w-6xl mx-auto px-6 py-4 flex items-baseline gap-6">
          <h1 className="text-slateblue-900">QCore</h1>
          <span className="text-sm text-slateblue-500">
            cellular dev &amp; test environment
          </span>
        </div>
        <nav className="max-w-6xl mx-auto px-6 flex gap-2">
          {TABS.map((t) => (
            <button
              key={t.id}
              className={`tab ${tab === t.id ? "tab-active" : ""}`}
              onClick={() => setTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </nav>
      </header>

      <main className="max-w-6xl mx-auto px-6 py-6">
        {tab === "health" && <HealthView onStartSim={() => setTab("trace")} />}
        {tab === "subscribers" && <SubscribersView />}
        {tab === "trace" && <LiveTraceView />}
        {tab === "ran" && <RANConnectView />}
      </main>
    </div>
  );
}
