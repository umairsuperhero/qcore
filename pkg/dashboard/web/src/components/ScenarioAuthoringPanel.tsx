import { useEffect, useState } from "react";
import {
  CheckCircle2,
  ExternalLink,
  FileText,
  Loader2,
  Play,
  Save,
  Trash2,
  XCircle,
} from "lucide-react";
import { api } from "../api/client";
import type { ScenarioRunResult, ScenarioSummary } from "../api/types";
import { useConnectionStore } from "../stores/connectionStore";

interface ScenarioAuthoringPanelProps {
  onStartTrace: () => void;
}

const WRONG_KI_TEMPLATE = `name: wrong_ki_check
description: 5G authentication mismatch should diagnose wrong Ki.
mode: 5g
scenario: wrong_ki
expect:
  result: failure
  cause: wrong_ki
  failed_step: security_mode_command
`;

const HAPPY_PATH_TEMPLATE = `name: fiveg_happy_path
description: 5G registration should complete.
mode: 5g
expect:
  result: success
`;

export default function ScenarioAuthoringPanel({ onStartTrace }: ScenarioAuthoringPanelProps) {
  const runSavedScenario = useConnectionStore((s) => s.runSavedScenario);
  const streaming = useConnectionStore((s) => s.traceState.streaming);
  const [items, setItems] = useState<ScenarioSummary[]>([]);
  const [yaml, setYaml] = useState(WRONG_KI_TEMPLATE);
  const [activeName, setActiveName] = useState("wrong_ki_check");
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ScenarioRunResult | null>(null);

  const refresh = async () => {
    const next = await api.listScenarios();
    setItems(next);
  };

  useEffect(() => {
    refresh().catch((e) => setError((e as Error).message));
  }, []);

  const save = async () => {
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const rec = await api.saveScenario(yaml);
      setActiveName(rec.name);
      setYaml(rec.yaml);
      setMessage(`Saved ${rec.name}`);
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const load = async (name: string) => {
    setError(null);
    setMessage(null);
    try {
      const rec = await api.getScenario(name);
      setActiveName(rec.name);
      setYaml(rec.yaml);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const remove = async (name: string) => {
    setError(null);
    setMessage(null);
    try {
      await api.deleteScenario(name);
      if (activeName === name) {
        setActiveName("");
      }
      setResult(null);
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const run = async (name: string) => {
    setRunning(name);
    setError(null);
    setMessage(null);
    setResult(null);
    try {
      const execution = runSavedScenario(name);
      onStartTrace();
      const next = await execution;
      setResult(next);
      setMessage(`${name}: ${formatVerdict(next)}`);
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setRunning(null);
    }
  };

  return (
    <div className="card mt-6 w-full max-w-xl bg-darkbg-900 border-darkbg-700/80 p-5">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-sky-500/10 text-sky-300 rounded-lg border border-sky-500/20">
            <FileText className="w-4 h-4" />
          </div>
          <div>
            <h2 className="text-base font-bold text-white">Scenarios</h2>
            <p className="text-xs text-slate-400 mt-1">Saved simulator checks.</p>
          </div>
        </div>
        <div className="flex gap-2">
          <button
            className="btn-secondary text-[11px]"
            disabled={saving || streaming}
            onClick={() => {
              setYaml(HAPPY_PATH_TEMPLATE);
              setActiveName("fiveg_happy_path");
            }}
          >
            Happy template
          </button>
          <button
            className="btn-secondary text-[11px]"
            disabled={saving || streaming}
            onClick={() => {
              setYaml(WRONG_KI_TEMPLATE);
              setActiveName("wrong_ki_check");
            }}
          >
            Wrong Ki template
          </button>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-1 lg:grid-cols-[1fr_1.1fr] gap-3">
        <div className="space-y-2">
          {items.length === 0 ? (
            <div className="rounded-lg border border-dashed border-darkbg-700 bg-darkbg-950/30 p-4 text-xs text-slate-500">
              No saved scenarios.
            </div>
          ) : (
            items.map((item) => (
              <div
                key={item.name}
                className={`rounded-lg border p-3 transition ${
                  activeName === item.name
                    ? "border-sky-500/40 bg-sky-500/5"
                    : "border-darkbg-800 bg-darkbg-950/30"
                }`}
              >
                <div className="flex items-start justify-between gap-2">
                  <button className="min-w-0 text-left" onClick={() => load(item.name)}>
                    <div className="font-mono text-xs font-bold text-white truncate">{item.name}</div>
                    <div className="mt-1 text-[11px] text-slate-400 truncate">
                      {(item.mode || "4g").toUpperCase()} · {item.expect ? expectLabel(item) : "raw outcome"}
                    </div>
                  </button>
                  <div className="flex gap-1">
                    <button
                      className="p-1.5 rounded-md border border-darkbg-700 text-emerald-400 hover:bg-emerald-500/10 disabled:opacity-50"
                      disabled={streaming || running !== null}
                      title="Run scenario"
                      onClick={() => run(item.name)}
                    >
                      {running === item.name ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5 fill-current" />}
                    </button>
                    <button
                      className="p-1.5 rounded-md border border-darkbg-700 text-slate-400 hover:text-red-300 hover:bg-red-500/10 disabled:opacity-50"
                      disabled={streaming || running !== null}
                      title="Delete scenario"
                      onClick={() => remove(item.name)}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>

        <div>
          <textarea
            className="min-h-[270px] w-full resize-y rounded-lg border border-darkbg-800 bg-darkbg-950/80 p-3 font-mono text-xs leading-relaxed text-slate-200 outline-none focus:border-sky-500/60"
            spellCheck={false}
            value={yaml}
            onChange={(e) => setYaml(e.target.value)}
          />
          <div className="mt-3 flex flex-wrap gap-2">
            <button
              className="btn-primary text-xs flex items-center justify-center gap-1.5 font-bold"
              disabled={saving || streaming || !yaml.trim()}
              onClick={save}
            >
              {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
              Save
            </button>
            <button
              className="btn-secondary text-xs flex items-center justify-center gap-1.5"
              disabled={streaming || running !== null || !activeName}
              onClick={() => run(activeName)}
            >
              {running === activeName ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5 fill-current" />}
              Run saved
            </button>
            {result?.journey_id && (
              <button
                className="btn-secondary text-xs flex items-center justify-center gap-1.5"
                onClick={onStartTrace}
              >
                <ExternalLink className="w-3.5 h-3.5" />
                Open trace
              </button>
            )}
          </div>
        </div>
      </div>

      {message && (
        <div className="mt-4 rounded-lg border border-emerald-500/25 bg-emerald-500/10 p-3 text-xs text-emerald-300">
          {message}
        </div>
      )}
      {error && (
        <div className="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-300 font-mono">
          {error}
        </div>
      )}
      {result && (
        <div
          className={`mt-4 rounded-lg border p-4 ${
            result.pass === true
              ? "border-emerald-500/25 bg-emerald-500/10"
              : result.pass === false
                ? "border-red-500/30 bg-red-500/10"
                : "border-slate-700 bg-darkbg-950/30"
          }`}
        >
          <div className="flex items-center gap-2 text-sm font-bold text-white">
            {result.pass === true ? (
              <CheckCircle2 className="w-4 h-4 text-emerald-400" />
            ) : result.pass === false ? (
              <XCircle className="w-4 h-4 text-red-300" />
            ) : (
              <FileText className="w-4 h-4 text-slate-400" />
            )}
            {formatVerdict(result)}
          </div>
          <div className="mt-2 grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs font-mono">
            <MiniStat label="Expected" value={result.expected ? expectText(result.expected) : "raw outcome"} />
            <MiniStat label="Actual" value={actualText(result)} />
            <MiniStat label="Failed step" value={result.failed_step || "none"} />
            <MiniStat label="Journey" value={result.journey_id || "none"} />
          </div>
        </div>
      )}
    </div>
  );
}

function formatVerdict(result: ScenarioRunResult) {
  if (result.pass === true) return "PASS";
  if (result.pass === false) return "FAIL";
  return "RAW OUTCOME";
}

function expectLabel(item: ScenarioSummary) {
  if (!item.expect) return "raw outcome";
  return `${item.expect.result}${item.expect.cause ? `:${item.expect.cause}` : ""}`;
}

function expectText(expect: NonNullable<ScenarioSummary["expect"]>) {
  return `${expect.result}${expect.cause ? ` / ${expect.cause}` : ""}${expect.failed_step ? ` / ${expect.failed_step}` : ""}`;
}

function actualText(result: ScenarioRunResult) {
  return `${result.actual.result}${result.actual.cause ? ` / ${result.actual.cause}` : ""}`;
}

function MiniStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-darkbg-800 bg-darkbg-950/50 p-2 min-w-0">
      <div className="text-slate-500 uppercase text-[10px] mb-1">{label}</div>
      <div className="text-slate-200 break-words">{value}</div>
    </div>
  );
}
