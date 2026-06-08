import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { CoreHealth, NFStatus, SimulatorStatus } from "../api/types";

interface Props {
  onStartSim: () => void;
}

const SCENARIOS = [
  { id: "wrong_ki", label: "Wrong Ki" },
  { id: "wrong_plmn", label: "Wrong PLMN" },
  { id: "unprovisioned_imsi", label: "Unknown IMSI" },
  { id: "wrong_mme_address", label: "Timeout" },
] as const;

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

  const [mode, setMode] = useState<"4g" | "5g">("5g");

  const startSim = async () => {
    setBusy(true);
    try {
      const status = await api.simulatorStart(mode);
      setSim(status);
      onStartSim();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const injectScenario = async (scenario: string) => {
    setBusy(true);
    try {
      const status = await api.simulatorInject(mode, scenario);
      setSim(status);
      onStartSim();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (err && !health) {
    return (
      <div className="card border-red-500/30 bg-red-500/10 text-red-400">
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
          health.ready_for_use ? "border-emerald-500/30 bg-emerald-500/10" : "border-amber-500/30 bg-amber-500/10"
        }`}
      >
        <div>
          <h2 className={health.ready_for_use ? "text-emerald-400" : "text-amber-400"}>
            {health.ready_for_use ? "Core is up and ready." : "Core is not fully up."}
          </h2>
          <p className="text-sm mt-1 text-slateblue-700">
            {health.ready_for_use
              ? "Start the built-in simulator to run your first attach."
              : "Wait for all network functions to report healthy. If this persists, check the logs."}
          </p>
          {health.ready_for_use && (
            <div className="mt-4 flex gap-4 text-sm font-medium">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="sim_mode"
                  value="4g"
                  checked={mode === "4g"}
                  onChange={() => setMode("4g")}
                />
                4G EPC
              </label>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="sim_mode"
                  value="5g"
                  checked={mode === "5g"}
                  onChange={() => setMode("5g")}
                />
                5G SA
              </label>
            </div>
          )}
        </div>
        <button
          className="btn-primary"
          disabled={!health.ready_for_use || busy || sim?.state === "running"}
          onClick={startSim}
        >
          {sim?.state === "running" ? "Simulator running…" : "Start simulator"}
        </button>
      </div>

      <section className="grid grid-cols-1 lg:grid-cols-[1fr_1.2fr] gap-4">
        <div className="card">
          <h2 className="mb-3">Simulator scenarios</h2>
          <div className="grid grid-cols-2 gap-2">
            {SCENARIOS.map((scenario) => (
              <button
                key={scenario.id}
                className="btn-secondary text-sm"
                disabled={!health.ready_for_use || busy || sim?.state === "running"}
                onClick={() => injectScenario(scenario.id)}
              >
                {scenario.label}
              </button>
            ))}
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between gap-3">
            <h2>Simulator status</h2>
            <span
              className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
                sim?.state === "success"
                  ? "bg-emerald-500/15 text-emerald-400"
                  : sim?.state === "failed"
                    ? "bg-red-500/15 text-red-400"
                    : sim?.state === "running"
                      ? "bg-amber-500/15 text-amber-400"
                      : "bg-slate-500/10 text-slate-400"
              }`}
            >
              {sim?.state ?? "idle"}
            </span>
          </div>
          <dl className="mt-3 grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-2 text-xs">
            <StatusRow label="Mode" value={(sim?.mode ?? mode).toUpperCase()} />
            <StatusRow label="Scenario" value={sim?.last_scenario || "Happy path"} />
            <StatusRow label="Failed step" value={sim?.failed_step || "None"} danger={Boolean(sim?.failed_step)} />
            <StatusRow label="Journey" value={sim?.last_journey || "None yet"} mono />
          </dl>
          {sim?.last_error && (
            <div className="mt-3 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-300">
              {sim.last_error}
            </div>
          )}
        </div>
      </section>

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

function StatusRow({
  label,
  value,
  danger = false,
  mono = false,
}: {
  label: string;
  value: string;
  danger?: boolean;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-slateblue-500">{label}</dt>
      <dd className={`${mono ? "font-mono" : ""} ${danger ? "text-red-300" : "text-slate-200"} truncate`}>
        {value}
      </dd>
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
              ? "bg-emerald-500/15 text-emerald-400"
              : "bg-red-500/15 text-red-400"
          }`}
        >
          {nf.reachable ? "up" : "down"}
        </span>
      </div>
      <div className="mt-2 text-xs text-slateblue-500">
        {nf.reachable ? (
          <>responded in {nf.latency_ms} ms</>
        ) : (
          <span className="text-red-400">{nf.error || "no response"}</span>
        )}
      </div>
    </div>
  );
}
