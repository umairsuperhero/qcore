import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, CheckCircle2, FileCheck2, RefreshCw } from "lucide-react";
import { api } from "../api/client";
import type { RANConfig, RANReconcileResult } from "../api/types";

interface RANConfigCheckPanelProps {
  config: RANConfig | null;
}

export default function RANConfigCheckPanel({ config }: RANConfigCheckPanelProps) {
  const defaults = useMemo(() => defaultYaml(config), [config]);
  const [gnbYaml, setGnbYaml] = useState("");
  const [ueYaml, setUeYaml] = useState("");
  const [checking, setChecking] = useState(false);
  const [result, setResult] = useState<RANReconcileResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!config) return;
    setGnbYaml((current) => current || defaults.gnb);
    setUeYaml((current) => current || defaults.ue);
  }, [config, defaults.gnb, defaults.ue]);

  const runCheck = async () => {
    setChecking(true);
    setError(null);
    try {
      const next = await api.reconcileRANConfig(gnbYaml, ueYaml);
      setResult(next);
    } catch (e) {
      setResult(null);
      setError((e as Error).message);
    } finally {
      setChecking(false);
    }
  };

  return (
    <div className="card mt-6 w-full max-w-xl bg-darkbg-900 border-darkbg-700/80 p-5">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-emerald-500/10 text-emerald-400 rounded-lg border border-emerald-500/20">
            <FileCheck2 className="w-4 h-4" />
          </div>
          <div>
            <h2 className="text-base font-bold text-white">Check my RAN config</h2>
            <p className="text-xs text-slate-400 mt-1">Static diff against the running core.</p>
          </div>
        </div>
        <button
          className="btn-primary text-xs flex items-center justify-center gap-1.5 font-bold"
          disabled={checking || !gnbYaml.trim() || !ueYaml.trim()}
          onClick={runCheck}
        >
          {checking ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <FileCheck2 className="w-3.5 h-3.5" />}
          Compare
        </button>
      </div>

      <div className="mt-4 grid grid-cols-1 lg:grid-cols-2 gap-3">
        <label className="block">
          <span className="text-[11px] font-bold uppercase tracking-widest text-slate-500">gNB YAML</span>
          <textarea
            className="mt-2 min-h-[190px] w-full resize-y rounded-lg border border-darkbg-800 bg-darkbg-950/80 p-3 font-mono text-xs leading-relaxed text-slate-200 outline-none focus:border-emerald-500/60"
            spellCheck={false}
            value={gnbYaml}
            onChange={(e) => setGnbYaml(e.target.value)}
          />
        </label>
        <label className="block">
          <span className="text-[11px] font-bold uppercase tracking-widest text-slate-500">UE YAML</span>
          <textarea
            className="mt-2 min-h-[190px] w-full resize-y rounded-lg border border-darkbg-800 bg-darkbg-950/80 p-3 font-mono text-xs leading-relaxed text-slate-200 outline-none focus:border-emerald-500/60"
            spellCheck={false}
            value={ueYaml}
            onChange={(e) => setUeYaml(e.target.value)}
          />
        </label>
      </div>

      {error && (
        <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-950/10 p-3 text-xs text-amber-300 font-mono">
          {error}
        </div>
      )}

      {result && (
        <div className="mt-4 space-y-3">
          {result.ok ? (
            <div className="flex items-start gap-3 rounded-lg border border-emerald-500/25 bg-emerald-500/10 p-4">
              <CheckCircle2 className="w-5 h-5 text-emerald-400 mt-0.5" />
              <div>
                <div className="text-sm font-bold text-emerald-300">Matches the core</div>
                <div className="text-xs text-slate-300 mt-1">{result.summary}</div>
              </div>
            </div>
          ) : (
            <>
              <div className="text-xs font-bold uppercase tracking-widest text-amber-400">
                {result.summary}
              </div>
              {(result.mismatches || []).map((item) => (
                <div
                  key={`${item.field}-${item.cause}`}
                  className="rounded-lg border border-amber-500/25 bg-amber-950/10 p-4"
                >
                  <div className="flex items-start gap-3">
                    <AlertTriangle className="w-5 h-5 text-amber-400 mt-0.5" />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-bold text-white">{item.field}</span>
                        <span className="rounded bg-darkbg-950 px-2 py-0.5 text-[10px] font-mono uppercase text-amber-300 border border-amber-500/20">
                          {item.cause}
                        </span>
                      </div>
                      <div className="mt-2 grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs font-mono">
                        <div className="rounded border border-darkbg-800 bg-darkbg-950/50 p-2">
                          <div className="text-slate-500 uppercase text-[10px] mb-1">Provided</div>
                          <div className="text-slate-200 break-words">{item.ran_value}</div>
                        </div>
                        <div className="rounded border border-darkbg-800 bg-darkbg-950/50 p-2">
                          <div className="text-slate-500 uppercase text-[10px] mb-1">QCore</div>
                          <div className="text-slate-200 break-words">{item.core_value}</div>
                        </div>
                      </div>
                      <div className="mt-3 text-xs text-slate-300 leading-relaxed">{item.fix}</div>
                    </div>
                  </div>
                </div>
              ))}
            </>
          )}
        </div>
      )}
    </div>
  );
}

function defaultYaml(config: RANConfig | null) {
  if (!config) {
    return { gnb: "", ue: "" };
  }
  const plmn = config.amf_plmn || config.plmn || "00101";
  const mcc = plmn.slice(0, 3);
  const mnc = plmn.slice(3) || "01";
  const sub = config.sample_subscriber;
  const dnn = sub.apn || "internet";
  return {
    gnb:
      config.config_snippet?.ueransim_gnb ||
      [
        `mcc: '${mcc}'`,
        `mnc: '${mnc}'`,
        "nci: '0x00000000001'",
        "idLength: 32",
        `tac: ${config.amf_tac || config.tac || 1}`,
        "slices:",
        "  - sst: 1",
        "    sd: '000001'",
      ].join("\n"),
    ue: [
      `supi: 'imsi-${sub.imsi}'`,
      `mcc: '${mcc}'`,
      `mnc: '${mnc}'`,
      `key: '${sub.ki}'`,
      `op: '${sub.opc}'`,
      "opType: 'OPC'",
      "protectionScheme: 0",
      "sessions:",
      "  - type: 'IPv4'",
      `    apn: '${dnn}'`,
      "    slice:",
      "      sst: 1",
      "      sd: '000001'",
      "configured-nssai:",
      "  - sst: 1",
      "    sd: '000001'",
      "default-nssai:",
      "  - sst: 1",
      "    sd: '000001'",
    ].join("\n"),
  };
}
