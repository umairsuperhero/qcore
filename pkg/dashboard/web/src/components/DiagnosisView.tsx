import { useMemo, useState } from "react";
import { AlertTriangle, ArrowRight, Check, Copy, RefreshCw } from "lucide-react";
import type { QEvent } from "../api/types";
import { useConnectionStore } from "../stores/connectionStore";

interface DiagnosisViewProps {
  onBack: () => void;
  onOpenTrace: () => void;
}

export default function DiagnosisView({ onBack, onOpenTrace }: DiagnosisViewProps) {
  const connection = useConnectionStore((s) => s.connection);
  const traceState = useConnectionStore((s) => s.traceState);
  const startSimulator = useConnectionStore((s) => s.startSimulator);
  const [copied, setCopied] = useState<"gnb" | "core" | null>(null);
  const [retrying, setRetrying] = useState(false);

  const failureEvent = useMemo(
    () => [...traceState.events].reverse().find((event) => event.severity === "error"),
    [traceState.events],
  );
  const rawEvents = useMemo(() => traceState.events.slice(-6), [traceState.events]);
  const diagnostic = traceState.diagnostic;
  const rootCause = diagnostic?.RootCause || connection.failReason || eventCause(failureEvent) || "Configuration mismatch";
  const explanation =
    diagnostic?.Explanation ||
    failureEvent?.message ||
    `QCore rejected setup because the RAN values do not match the configured core identity.`;
  const fix =
    diagnostic?.Fix ||
    connection.fixGnb ||
    `Set the RAN PLMN to ${connection.configuredPlmn} and TAC to ${connection.configuredTac}.`;
  const gnbSnippet = [
    `mcc: "${connection.configuredPlmn.slice(0, 3)}"`,
    `mnc: "${connection.configuredPlmn.slice(3) || "01"}"`,
    `tac: ${connection.configuredTac}`,
  ].join("\n");
  const coreSnippet = [
    `plmn: "${connection.sentPlmn || connection.configuredPlmn}"`,
    `tac: ${connection.negotiatedTac || connection.configuredTac}`,
  ].join("\n");

  const copyText = async (key: "gnb" | "core", text: string) => {
    await navigator.clipboard.writeText(text);
    setCopied(key);
    window.setTimeout(() => setCopied(null), 1800);
  };

  const retry = async () => {
    setRetrying(true);
    try {
      await startSimulator();
      onOpenTrace();
    } finally {
      setRetrying(false);
    }
  };

  return (
    <div className="qdiag-stage">
      <header className="qdiag-topbar">
        <button type="button" onClick={onBack}>
          ← dashboard
        </button>
        <span>QCore diagnosis</span>
      </header>

      <main className="qdiag-shell">
        <section className="qdiag-summary">
          <span className="qdiag-kicker">Connection blocked</span>
          <div className="qdiag-title-row">
            <AlertTriangle className="h-8 w-8" />
            <h1>{rootCause}</h1>
          </div>
          <p>{explanation}</p>
        </section>

        <section className="qdiag-compare" aria-label="Configuration mismatch">
          <div>
            <span>RAN sent</span>
            <b>{connection.sentPlmn || "unknown"}</b>
            <small>{failureEvent?.nf ? `${failureEvent.nf.toUpperCase()} log evidence` : "from live trace"}</small>
          </div>
          <ArrowRight className="h-5 w-5" />
          <div>
            <span>QCore expects</span>
            <b>{connection.configuredPlmn}</b>
            <small>TAC {connection.configuredTac}</small>
          </div>
        </section>

        <section className="qdiag-fixes">
          <article>
            <div>
              <span>Fix on gNB</span>
              <h2>Make the RAN match QCore</h2>
              <p>{fix}</p>
            </div>
            <pre>{gnbSnippet}</pre>
            <button type="button" onClick={() => copyText("gnb", gnbSnippet)}>
              {copied === "gnb" ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              Copy gNB config
            </button>
          </article>

          <article>
            <div>
              <span>Fix on QCore</span>
              <h2>Or align the core to this RAN</h2>
              <p>Copy the core-side values, review them in your config, then restart the stack.</p>
            </div>
            <pre>{coreSnippet}</pre>
            <button type="button" onClick={() => copyText("core", coreSnippet)}>
              {copied === "core" ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              Copy core config
            </button>
          </article>
        </section>

        <section className="qdiag-actions">
          <button type="button" className="qhero-btn qhero-btn-primary" onClick={retry} disabled={retrying}>
            {retrying ? <RefreshCw className="h-4 w-4 animate-spin" /> : null}
            Retry setup
          </button>
          <button type="button" className="qhero-btn qhero-btn-ghost" onClick={onOpenTrace}>
            Open raw trace
          </button>
        </section>

        <details className="qdiag-raw">
          <summary>Raw signaling evidence</summary>
          {rawEvents.length === 0 ? (
            <pre>No live trace events have arrived yet.</pre>
          ) : (
            rawEvents.map((event) => <pre key={event.id}>{formatRaw(event)}</pre>)
          )}
        </details>
      </main>
    </div>
  );
}

function eventCause(event?: QEvent) {
  const cause = event?.payload?.cause || event?.payload?.message;
  return typeof cause === "string" ? cause : "";
}

function formatRaw(event: QEvent) {
  return `${event.timestamp} ${event.nf.toUpperCase()} ${event.severity.toUpperCase()} ${event.message}\n${JSON.stringify(
    event.payload || {},
    null,
    2,
  )}`;
}
