import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { RANConfig } from "../api/types";

const FORMATS: { id: string; label: string }[] = [
  { id: "open5gs_enb", label: "Generic gNB / eNB" },
  { id: "srsran_enb", label: "srsRAN enb.conf" },
];

export default function RANConnectView() {
  const [cfg, setCfg] = useState<RANConfig | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [format, setFormat] = useState(FORMATS[0].id);

  useEffect(() => {
    api.ranConfig().then(setCfg).catch((e) => setErr(e.message));
  }, []);

  if (err) return <div className="card border-red-200 bg-red-50 text-red-800">{err}</div>;
  if (!cfg) return <div className="text-slateblue-500">Loading…</div>;

  const snippet = cfg.config_snippet[format] || "";

  return (
    <div className="space-y-6">
      <div>
        <h2>Connect a real RAN</h2>
        <p className="text-sm text-slateblue-500 mt-1">
          Point a gNodeB, eNB, or device at QCore using these values. They are
          read from the running core's config — change the core's config to
          change them.
        </p>
      </div>

      <section className="card">
        <h2 className="mb-4">Core values</h2>
        <dl className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-3 text-sm">
          <KV k="PLMN" v={cfg.plmn} />
          <KV k="TAC" v={String(cfg.tac)} />
          <KV k="MME address" v={`${cfg.mme_address}:${cfg.mme_s1ap_port}`} />
          <KV k="MME Group ID / Code" v={`${cfg.mme_group_id} / ${cfg.mme_code}`} />
          <KV k="SGW S1-U (GTP-U)" v={`${cfg.spgw_gtpu_address}:${cfg.spgw_gtpu_port}`} />
          <KV k="UE address pool" v={cfg.ue_pool} />
        </dl>
        {cfg.mme_address.startsWith("<") && (
          <p className="text-xs text-amber-700 mt-3">
            The MME bind address is <code>0.0.0.0</code>. Substitute the actual
            host IP your RAN can reach.
          </p>
        )}
      </section>

      <section className="card">
        <h2 className="mb-4">Sample SIM</h2>
        <p className="text-xs text-slateblue-500 mb-3">
          Seeded automatically on first run. These are the 3GPP TS 35.208 Test Set 1
          vectors — published, not secrets.
        </p>
        <dl className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-3 text-sm font-mono">
          <KV k="IMSI" v={cfg.sample_subscriber.imsi} />
          <KV k="Ki" v={cfg.sample_subscriber.ki} />
          <KV k="OPc" v={cfg.sample_subscriber.opc} />
          <KV k="AMF" v={cfg.sample_subscriber.amf} />
        </dl>
      </section>

      <section className="card">
        <div className="flex items-center justify-between mb-3">
          <h2>Copy-paste config</h2>
          <div className="flex gap-2">
            {FORMATS.map((f) => (
              <button
                key={f.id}
                className={`text-xs px-2 py-1 rounded ${
                  format === f.id
                    ? "bg-slateblue-700 text-white"
                    : "border border-slateblue-100 text-slateblue-700 hover:bg-slateblue-50"
                }`}
                onClick={() => setFormat(f.id)}
              >
                {f.label}
              </button>
            ))}
            <button
              className="text-xs px-2 py-1 rounded border border-slateblue-100 text-slateblue-700 hover:bg-slateblue-50"
              onClick={() => navigator.clipboard.writeText(snippet)}
            >
              Copy
            </button>
          </div>
        </div>
        <pre className="bg-slateblue-50 rounded p-3 text-xs overflow-x-auto font-mono whitespace-pre">
          {snippet}
        </pre>
      </section>
    </div>
  );
}

function KV({ k, v }: { k: string; v: string }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wider text-slateblue-500">{k}</dt>
      <dd className="font-mono mt-0.5">{v}</dd>
    </div>
  );
}
