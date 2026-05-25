import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { CoreHealth, NFStatus, SimulatorStatus } from "../api/types";

interface Props {
  onStartSim: () => void;
}

export default function HealthView({ onStartSim }: Props) {
  const [health, setHealth] = useState<CoreHealth | null>(null);
  const [sim, setSim] = useState<SimulatorStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const tick = async () => {
      try {
        const [h, s] = await Promise.all([api.health(), api.simulatorStatus()]);
        setHealth(h);
        setSim(s);
        setErr(null);
      } catch (e) {
        setErr((e as Error).message);
      }
    };
    tick();
    const t = setInterval(tick, 3000);
    return () => clearInterval(t);
  }, []);

  const startSim = async () => {
    setBusy(true);
    try {
      await api.simulatorStart();
      onStartSim();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (err && !health) {
    return (
      <div className="card border-red-200 bg-red-50 text-red-800">
        Dashboard backend unreachable: {err}
      </div>
    );
  }
  if (!health) {
    return <div className="text-slateblue-500">Loading…</div>;
  }

  return (
    <div className="space-y-6">
      <div
        className={`card flex items-center justify-between ${
          health.ready_for_use ? "border-emerald-200 bg-emerald-50" : "border-amber-200 bg-amber-50"
        }`}
      >
        <div>
          <h2 className={health.ready_for_use ? "text-emerald-800" : "text-amber-800"}>
            {health.ready_for_use ? "Core is up and ready." : "Core is not fully up."}
          </h2>
          <p className="text-sm mt-1 text-slateblue-700">
            {health.ready_for_use
              ? "Start the built-in simulator to run your first attach."
              : "Wait for all network functions to report healthy. If this persists, check the logs."}
          </p>
        </div>
        <button
          className="btn-primary"
          disabled={!health.ready_for_use || busy || sim?.state === "running"}
          onClick={startSim}
        >
          {sim?.state === "running" ? "Simulator running…" : "Start simulator"}
        </button>
      </div>

      <section>
        <h2 className="mb-3">Network functions</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {health.nfs.map((nf) => (
            <NFCard key={nf.name} nf={nf} />
          ))}
        </div>
      </section>
    </div>
  );
}

function NFCard({ nf }: { nf: NFStatus }) {
  return (
    <div className="card">
      <div className="flex items-center justify-between">
        <div>
          <div className="font-semibold uppercase text-sm tracking-wide">
            {nf.name}
          </div>
          <div className="text-xs text-slateblue-500 font-mono mt-0.5 break-all">
            {nf.url}
          </div>
        </div>
        <span
          className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
            nf.reachable
              ? "bg-emerald-100 text-emerald-800"
              : "bg-red-100 text-red-800"
          }`}
        >
          {nf.reachable ? "up" : "down"}
        </span>
      </div>
      <div className="mt-2 text-xs text-slateblue-500">
        {nf.reachable ? (
          <>responded in {nf.latency_ms} ms</>
        ) : (
          <span className="text-red-700">{nf.error || "no response"}</span>
        )}
      </div>
    </div>
  );
}
