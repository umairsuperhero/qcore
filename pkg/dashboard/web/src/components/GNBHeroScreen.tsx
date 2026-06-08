import { useState } from "react";
import { motion } from "framer-motion";
import { 
  CheckCircle2, 
  AlertTriangle, 
  ArrowRight, 
  Copy, 
  Cpu, 
  Wifi,
  RefreshCw,
  Check,
  Play
} from "lucide-react";
import { useConnectionStore } from "../stores/connectionStore";

interface GNBHeroScreenProps {
  onRegisterUE: () => void;
  onStartTrace: () => void;
}

const SCENARIOS = [
  { id: "wrong_ki", label: "Wrong Ki" },
  { id: "wrong_plmn", label: "Wrong PLMN" },
  { id: "unprovisioned_imsi", label: "Unknown IMSI" },
  { id: "wrong_mme_address", label: "Timeout" },
] as const;

export default function GNBHeroScreen({ onRegisterUE, onStartTrace }: GNBHeroScreenProps) {
  const connection = useConnectionStore((s) => s.connection);
  const loading = useConnectionStore((s) => s.loading);
  const mode = useConnectionStore((s) => s.mode);
  const setMode = useConnectionStore((s) => s.setMode);
  const startSimulator = useConnectionStore((s) => s.startSimulator);
  const streaming = useConnectionStore((s) => s.traceState.streaming);

  const [copied, setCopied] = useState(false);
  const [startingScenario, setStartingScenario] = useState<string | null>(null);

  const copyAddress = () => {
    navigator.clipboard.writeText(connection.amfAddress);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const launchSimulator = async (scenario?: string) => {
    const key = scenario || "happy_path";
    setStartingScenario(key);
    try {
      await startSimulator(scenario);
      onStartTrace();
    } finally {
      setStartingScenario(null);
    }
  };

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[60vh] text-slate-400">
        <RefreshCw className="w-8 h-8 animate-spin text-emerald-500 mb-4" />
        <p className="text-sm font-medium font-mono">Querying core configuration...</p>
      </div>
    );
  }

  // The 3D flip animates between "Waiting/Failed" (front) and "Connected" (back).
  // A card rotation is 180 degrees.
  const isConnected = connection.state === "connected";
  const modeTitle = mode === "5g" ? "gNB" : "eNB";
  const setupName = mode === "5g" ? "NG Setup" : "S1 Setup";
  const endpointLabel = mode === "5g" ? "AMF NGAP (SCTP)" : "MME S1AP (SCTP)";

  // Card shake variant for the failed state
  const shakeVariants = {
    shake: {
      x: [0, -10, 10, -10, 10, -5, 5, -2, 2, 0],
      transition: { duration: 0.6, ease: "easeInOut" } as any
    },
    idle: { x: 0 }
  };

  return (
    <div className="relative flex flex-col items-center justify-center min-h-[70vh] py-12 px-4 select-none">
      
      {/* Title Header */}
      <div className="text-center mb-10 max-w-xl">
        <span className="text-xs font-bold tracking-widest text-emerald-500 uppercase bg-emerald-500/10 px-3 py-1 rounded-full border border-emerald-500/20 mb-3 inline-block">
          Access Stratum (RAN)
        </span>
        <h1 className="text-4xl md:text-5xl font-extrabold tracking-tight bg-gradient-to-b from-white to-slate-400 bg-clip-text text-transparent">
          Is your {modeTitle} connected?
        </h1>
        <p className="text-slate-400 mt-3 text-sm md:text-base">
          Establish the interface (NG Setup) between your 5G gNodeB / 4G eNodeB and the Core.
        </p>
      </div>

      <div className="mb-6 bg-darkbg-950 p-1 rounded-lg border border-darkbg-800 flex">
        <button
          className={`text-xs px-4 py-2 rounded-md font-semibold transition ${
            mode === "4g"
              ? "bg-darkbg-900 text-emerald-400 shadow-sm border border-darkbg-700"
              : "text-slate-400 hover:text-white"
          }`}
          onClick={() => setMode("4g")}
        >
          4G EPC
        </button>
        <button
          className={`text-xs px-4 py-2 rounded-md font-semibold transition ${
            mode === "5g"
              ? "bg-darkbg-900 text-emerald-400 shadow-sm border border-darkbg-700"
              : "text-slate-400 hover:text-white"
          }`}
          onClick={() => setMode("5g")}
        >
          5G SA
        </button>
      </div>

      {/* Main 3D Flippable Container */}
      <div className="perspective-1000 w-full max-w-xl min-h-[420px] relative">
        <motion.div
          animate={{ rotateX: isConnected ? 180 : 0 }}
          transition={{ type: "spring", stiffness: 90, damping: 14, mass: 1.1 }}
          style={{ transformStyle: "preserve-3d" }}
          className="w-full h-full absolute top-0 left-0"
        >
          {/* FRONT SIDE: Waiting OR Failed */}
          <motion.div
            className="backface-hidden w-full h-full absolute top-0 left-0"
            animate={connection.state === "failed" ? "shake" : "idle"}
            variants={shakeVariants}
          >
            {connection.state === "failed" ? (
              /* FAILED STATE CARD */
              <div className="card w-full h-full min-h-[420px] bg-darkbg-900 border-amber-500/30 shadow-amber-950/10 flex flex-col justify-between p-8 relative overflow-hidden">
                {/* Visual Accent/Glow */}
                <div className="absolute top-0 right-0 w-48 h-48 bg-amber-500/5 rounded-full blur-3xl pointer-events-none" />
                
                {/* Header info */}
                <div>
                  <div className="flex items-center gap-3 mb-6">
                    <div className="p-2.5 bg-amber-500/10 text-amber-400 rounded-xl border border-amber-500/20">
                      <AlertTriangle className="w-6 h-6 animate-pulse" />
                    </div>
                    <div>
                      <h2 className="text-xl font-bold text-white tracking-tight">
                        {setupName} rejected
                      </h2>
                      <p className="text-xs text-amber-400/80 font-mono mt-0.5">
                        Cause: PLMN ID Mismatch
                      </p>
                    </div>
                  </div>

                  <p className="text-sm text-slate-300 leading-relaxed bg-darkbg-950/60 p-4 rounded-xl border border-darkbg-800 font-mono mb-6">
                    Your {modeTitle} sent PLMN <span className="text-amber-400 font-bold">{connection.sentPlmn}</span>. 
                    QCore is currently configured to expect <span className="text-emerald-400 font-bold">{connection.configuredPlmn}</span>.
                  </p>
                </div>

                {/* Resolution Plan */}
                <div className="space-y-4">
                  <div className="border-t border-darkbg-800/80 pt-4">
                    <div className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3">
                      Resolution Actions
                    </div>
                    
                    <div className="grid grid-cols-1 gap-3">
                      {/* Fix on gNB */}
                      <div className="p-4 bg-darkbg-950/40 rounded-xl border border-darkbg-850 flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                        <div>
                          <div className="text-xs font-semibold text-slate-400">Fix on gNB side</div>
                          <div className="text-sm font-mono text-slate-200 mt-1">
                            Set MCC/MNC parameters to match configuration.
                          </div>
                        </div>
                        <span className="text-xs font-mono text-amber-400/90 font-semibold bg-amber-500/10 px-2.5 py-1 rounded-md border border-amber-500/20 whitespace-nowrap self-start sm:self-auto">
                          {connection.fixGnb}
                        </span>
                      </div>

                      <div className="w-full text-left p-4 bg-emerald-500/5 rounded-xl border border-emerald-500/20 flex items-center justify-between gap-4">
                        <div>
                          <div className="text-xs font-semibold text-emerald-400">Fix on QCore side</div>
                          <div className="text-sm font-mono text-slate-200 mt-1">
                            Update QCore PLMN/TAC to match the RAN, then restart the stack.
                          </div>
                        </div>
                        <div className="flex items-center gap-1.5 text-xs text-emerald-400 font-semibold whitespace-nowrap">
                          Check config
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            ) : (
              /* WAITING STATE CARD */
              <div className="card w-full h-full min-h-[420px] bg-darkbg-900 border-darkbg-700/80 shadow-2xl flex flex-col justify-between p-8 relative overflow-hidden">
                {/* Pulse Glow Effect */}
                <div className="absolute top-0 right-0 w-64 h-64 bg-emerald-500/5 rounded-full blur-3xl animate-pulse pointer-events-none" />
                
                {/* Card Title & Pulse */}
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                      <span className="relative flex h-3.5 w-3.5">
                      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                      <span className="relative inline-flex rounded-full h-3.5 w-3.5 bg-emerald-500"></span>
                    </span>
                    <span className="text-sm text-slate-300 font-medium font-mono">
                      Waiting for {modeTitle} connection...
                    </span>
                  </div>
                  <Wifi className="w-5 h-5 text-slate-500" />
                </div>

                {/* Connection Parameters Section */}
                <div className="my-6">
                  <div className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3">
                    Point your gNB here
                  </div>
                  
                  <div className="space-y-3 bg-darkbg-950/70 p-5 rounded-2xl border border-darkbg-800 font-mono text-sm">
                    {/* AMF Address */}
                    <div className="flex items-center justify-between group border-b border-darkbg-800/40 pb-2">
                      <span className="text-slate-400">{endpointLabel}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-white font-semibold">{connection.amfAddress}</span>
                        <button 
                          onClick={copyAddress}
                          className="text-slate-500 hover:text-white p-1 hover:bg-darkbg-800 rounded transition"
                          title="Copy AMF Address"
                        >
                          {copied ? (
                            <Check className="w-4 h-4 text-emerald-400" />
                          ) : (
                            <Copy className="w-4 h-4" />
                          )}
                        </button>
                      </div>
                    </div>

                    {/* PLMN */}
                    <div className="flex items-center justify-between border-b border-darkbg-800/40 pb-2">
                      <span className="text-slate-400">PLMN</span>
                      <span className="text-white font-semibold">{connection.configuredPlmn}</span>
                    </div>

                    {/* TAC */}
                    <div className="flex items-center justify-between">
                      <span className="text-slate-400">TAC</span>
                      <span className="text-white font-semibold">{connection.configuredTac}</span>
                    </div>
                  </div>
                </div>

                {/* Quick Info */}
                <div className="flex items-center justify-between border-t border-darkbg-800/80 pt-5 gap-3">
                  <div className="flex items-center gap-2 text-xs text-slate-400 font-medium">
                    <Cpu className="w-4 h-4 text-emerald-500/80" />
                    <span>Zero-config deployment active.</span>
                  </div>
                </div>
              </div>
            )}
          </motion.div>

          {/* BACK SIDE: Connected State Card */}
          <motion.div
            style={{ 
              backfaceVisibility: "hidden",
              transform: "rotateX(180deg)" 
            }}
            className="w-full h-full absolute top-0 left-0"
          >
            <div className="card w-full h-full min-h-[420px] bg-darkbg-900 border-emerald-500/30 shadow-emerald-950/10 flex flex-col justify-between p-8 relative overflow-hidden animate-glow">
              {/* Radial Success Glow */}
              <div className="absolute top-0 right-0 w-64 h-64 bg-emerald-500/10 rounded-full blur-3xl pointer-events-none" />

              {/* Status Header */}
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-emerald-500/10 text-emerald-400 rounded-xl border border-emerald-500/20">
                    <CheckCircle2 className="w-6 h-6 animate-pulse" />
                  </div>
                  <div>
                    <h2 className="text-xl font-bold text-white tracking-tight">
                      gNB Connected
                    </h2>
                    <p className="text-xs text-emerald-400 font-mono mt-0.5">
                      Interface ({setupName}) established
                    </p>
                  </div>
                </div>
                <Wifi className="w-5 h-5 text-emerald-400" />
              </div>

              {/* Confirmed Telemetry Node Details */}
              <div className="my-6">
                <div className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3">
                  Device Information
                </div>

                <div className="space-y-4">
                  {/* Connected Device Identifier */}
                  <div className="bg-darkbg-950/60 p-4 rounded-xl border border-darkbg-800">
                    <div className="text-xs text-slate-400 font-semibold mb-1">gNB Identity</div>
                    <div className="text-base font-semibold text-white font-mono flex items-center gap-2">
                      <span className="w-2.5 h-2.5 rounded-full bg-emerald-400" />
                      {connection.gnbName || "Nokia AirScale"}
                    </div>
                  </div>

                  {/* Negotiated params bar */}
                  <div className="grid grid-cols-3 gap-2 text-center text-xs font-mono">
                    <div className="bg-darkbg-950/30 p-2.5 rounded-lg border border-darkbg-850">
                      <div className="text-[10px] text-slate-400 uppercase font-semibold mb-1">PLMN</div>
                      <div className="text-slate-200 font-bold">{connection.negotiatedPlmn}</div>
                    </div>
                    <div className="bg-darkbg-950/30 p-2.5 rounded-lg border border-darkbg-850">
                      <div className="text-[10px] text-slate-400 uppercase font-semibold mb-1">TAC</div>
                      <div className="text-slate-200 font-bold">{connection.negotiatedTac}</div>
                    </div>
                    <div className="bg-darkbg-950/30 p-2.5 rounded-lg border border-darkbg-850">
                      <div className="text-[10px] text-slate-400 uppercase font-semibold mb-1">Slice</div>
                      <div className="text-emerald-400 font-bold">{connection.negotiatedSlice}</div>
                    </div>
                  </div>
                </div>
              </div>

              {/* Dynamic Action Trigger to Next Step */}
              <button
                onClick={onRegisterUE}
                className="w-full btn-primary py-3 rounded-xl flex items-center justify-center gap-2 font-semibold text-sm group"
              >
                Register a UE
                <ArrowRight className="w-4 h-4 group-hover:translate-x-1 transition duration-200" />
              </button>
            </div>
          </motion.div>
        </motion.div>
      </div>

      <div className="card mt-6 w-full max-w-xl bg-darkbg-900 border-darkbg-700/80 p-5">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-bold text-white">Built-in simulator</h2>
            <p className="text-xs text-slate-400 mt-1">
              Start a real {mode.toUpperCase()} control-plane run and watch the SSE trace.
            </p>
          </div>
          <button
            className="btn-primary text-xs flex items-center justify-center gap-1.5 font-bold"
            disabled={streaming || startingScenario !== null}
            onClick={() => launchSimulator()}
          >
            {startingScenario === "happy_path" ? (
              <RefreshCw className="w-3.5 h-3.5 animate-spin" />
            ) : (
              <Play className="w-3.5 h-3.5 fill-current" />
            )}
            Happy path
          </button>
        </div>

        <div className="mt-4 grid grid-cols-2 sm:grid-cols-4 gap-2">
          {SCENARIOS.map((scenario) => (
            <button
              key={scenario.id}
              className="btn-secondary text-[11px] font-medium border-amber-950/20 text-amber-400 bg-amber-950/5 hover:bg-amber-950/20"
              disabled={streaming || startingScenario !== null}
              onClick={() => launchSimulator(scenario.id)}
            >
              {startingScenario === scenario.id ? "Starting..." : scenario.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
