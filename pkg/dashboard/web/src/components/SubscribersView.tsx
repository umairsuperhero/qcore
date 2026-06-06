import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Subscriber } from "../api/types";

const IMSI_RE = /^\d{14,15}$/;
const HEX32_RE = /^[0-9a-fA-F]{32}$/;
const HEX4_RE = /^[0-9a-fA-F]{4}$/;

interface FormState {
  imsi: string;
  ki: string;
  opc: string;
  amf: string;
}

const EMPTY_FORM: FormState = { imsi: "", ki: "", opc: "", amf: "8000" };

export default function SubscribersView() {
  const [subs, setSubs] = useState<Subscriber[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [submitErr, setSubmitErr] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const load = async () => {
    setLoading(true);
    try {
      const r = await api.listSubscribers();
      setSubs(r.data ?? []);
      setTotal(r.total ?? 0);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitErr(null);
    const v = validate(form);
    if (v) {
      setSubmitErr(v);
      return;
    }
    setSubmitting(true);
    try {
      await api.createSubscriber({
        imsi: form.imsi,
        ki: form.ki.toLowerCase(),
        opc: form.opc.toLowerCase(),
        amf: form.amf.toLowerCase(),
      });
      setForm(EMPTY_FORM);
      setShowForm(false);
      await load();
    } catch (e) {
      setSubmitErr((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const remove = async (imsi: string) => {
    if (!confirm(`Delete subscriber ${imsi}?`)) return;
    try {
      await api.deleteSubscriber(imsi);
      await load();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-baseline justify-between">
        <div>
          <h2>Subscribers</h2>
          <p className="text-sm text-slateblue-500 mt-1">
            {total} provisioned. Configure a SIM with these values, or use the
            seeded demo subscriber.
          </p>
        </div>
        <button
          className="btn-primary"
          onClick={() => {
            setShowForm((v) => !v);
            setSubmitErr(null);
          }}
        >
          {showForm ? "Cancel" : "Add subscriber"}
        </button>
      </div>

      {showForm && (
        <form className="card space-y-4" onSubmit={submit}>
          <Field
            label="IMSI"
            help="15 digits. MCC (3) + MNC (2 or 3) + MSIN. Example: 001010000000001."
            value={form.imsi}
            onChange={(v) => setForm({ ...form, imsi: v })}
            placeholder="001010000000001"
          />
          <Field
            label="Ki"
            help="128-bit subscriber key, hex. 32 hex chars."
            value={form.ki}
            onChange={(v) => setForm({ ...form, ki: v })}
            placeholder="465b5ce8b199b49faa5f0a2ee238a6bc"
          />
          <Field
            label="OPc"
            help="Operator key derived from Op + Ki. 32 hex chars."
            value={form.opc}
            onChange={(v) => setForm({ ...form, opc: v })}
            placeholder="cd63cb71954a9f4e48a5994e37a02baf"
          />
          <Field
            label="AMF"
            help="Authentication Management Field. 4 hex chars. Default 8000 works for most tests."
            value={form.amf}
            onChange={(v) => setForm({ ...form, amf: v })}
            placeholder="8000"
          />
          {submitErr && (
            <div className="text-sm text-red-400 bg-red-500/10 border border-red-500/30 rounded-md px-3 py-2">
              {submitErr}
            </div>
          )}
          <div className="flex justify-end">
            <button className="btn-primary" disabled={submitting}>
              {submitting ? "Creating…" : "Create subscriber"}
            </button>
          </div>
        </form>
      )}

      {err && (
        <div className="card border-red-500/30 bg-red-500/10 text-red-400">{err}</div>
      )}
      {loading ? (
        <div className="text-slateblue-500">Loading…</div>
      ) : subs.length === 0 ? (
        <div className="card text-slateblue-500">
          No subscribers yet. Add one above, or run with the default seeded demo.
        </div>
      ) : (
        <div className="card overflow-hidden p-0">
          <table className="w-full text-sm">
            <thead className="bg-darkbg-800 text-left text-xs uppercase tracking-wide text-slateblue-500">
              <tr>
                <th className="px-4 py-2">IMSI</th>
                <th className="px-4 py-2">Ki</th>
                <th className="px-4 py-2">OPc</th>
                <th className="px-4 py-2">APN</th>
                <th className="px-4 py-2"></th>
              </tr>
            </thead>
            <tbody className="font-mono">
              {subs.map((s) => (
                <tr key={s.imsi} className="border-t border-darkbg-700">
                  <td className="px-4 py-2">{s.imsi}</td>
                  <td className="px-4 py-2 text-slateblue-500">{mask(s.ki)}</td>
                  <td className="px-4 py-2 text-slateblue-500">{mask(s.opc)}</td>
                  <td className="px-4 py-2">{s.apn || "—"}</td>
                  <td className="px-4 py-2 text-right">
                    <button
                      className="btn-danger"
                      onClick={() => remove(s.imsi)}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

interface FieldProps {
  label: string;
  help: string;
  value: string;
  placeholder?: string;
  onChange: (v: string) => void;
}
function Field({ label, help, value, placeholder, onChange }: FieldProps) {
  return (
    <div>
      <label className="label">{label}</label>
      <input
        className="input"
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        autoComplete="off"
      />
      <p className="text-xs text-slateblue-500 mt-1">{help}</p>
    </div>
  );
}

function validate(f: FormState): string | null {
  if (!IMSI_RE.test(f.imsi)) return "IMSI must be 14 or 15 digits.";
  if (!HEX32_RE.test(f.ki)) return "Ki must be 32 hex characters.";
  if (!HEX32_RE.test(f.opc)) return "OPc must be 32 hex characters.";
  if (!HEX4_RE.test(f.amf)) return "AMF must be 4 hex characters.";
  return null;
}

function mask(hex: string): string {
  if (!hex || hex.length < 8) return "…";
  return hex.slice(0, 4) + "…" + hex.slice(-4);
}
