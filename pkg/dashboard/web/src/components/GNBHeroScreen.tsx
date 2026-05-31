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
  Sliders,
  Check
} from "lucide-react";
import { useGNBConnection } from "../api/gnbConnection";

interface GNBHeroScreenProps {
  onRegisterUE: () => void;
  onUseUeransim: () => void;
}

export default function GNBHeroScreen({ onRegisterUE, onUseUeransim }: GNBHeroScreenProps) {
  const { 
    connection, 
    loading, 
    isSimulated, 
    simulatedState, 
    triggerSimulation, 
    resetToLive 
  } = useGNBConnection();

  const [copied, setCopied] = useState(false);
  const [fixingQCore, setFixingQCore] = useState(false);

  const copyAddress = () => {
    navigator.clipboard.writeText(connection.amfAddress);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  // Perform mock fix for QCore configuration
  const handleFixQCore = () => {
    if (connection.state !== "failed") return;
    setFixingQCore(true);
    
    // Simulate updating the config and establishing a successful connection after 1.5s
    setTimeout(() => {
      setFixingQCore(false);
      triggerSimulation("connected");
    }, 1500);
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
          Is your gNB connected?
        </h1>
        <p className="text-slate-400 mt-3 text-sm md:text-base">
          Establish the interface (NG Setup) between your 5G gNodeB / 4G eNodeB and the Core.
        </p>
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
                        NG Setup rejected
                      </h2>
                      <p className="text-xs text-amber-400/80 font-mono mt-0.5">
                        Cause: PLMN ID Mismatch
                      </p>
                    </div>
                  </div>

                  <p className="text-sm text-slate-300 leading-relaxed bg-darkbg-950/60 p-4 rounded-xl border border-darkbg-800 font-mono mb-6">
                    Your gNB sent PLMN <span className="text-amber-400 font-bold">{connection.sentPlmn}</span>. 
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

                      {/* Fix on QCore */}
                      <button
                        onClick={handleFixQCore}
                        disabled={fixingQCore}
                        className="w-full text-left p-4 bg-emerald-500/5 hover:bg-emerald-500/10 disabled:bg-slate-900/30 rounded-xl border border-emerald-500/20 hover:border-emerald-500/40 transition group flex items-center justify-between gap-4"
                      >
                        <div>
                          <div className="text-xs font-semibold text-emerald-400">Fix on QCore side</div>
                          <div className="text-sm font-mono text-slate-200 mt-1">
                            Reconfigure QCore's expectation automatically.
                          </div>
                        </div>
                        <div className="flex items-center gap-1.5 text-xs text-emerald-400 font-semibold group-hover:translate-x-1 transition duration-200 whitespace-nowrap">
                          {fixingQCore ? (
                            <RefreshCw className="w-4 h-4 animate-spin" />
                          ) : (
                            <>
                              Change to {connection.sentPlmn}
                              <ArrowRight className="w-4 h-4" />
                            </>
                          )}
                        </div>
                      </button>
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
                      Waiting for gNB connection...
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
                      <span className="text-slate-400">AMF (SCTP)</span>
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

                {/* Quick Info & Action Link */}
                <div className="flex flex-col sm:flex-row items-center justify-between border-t border-darkbg-800/80 pt-5 gap-3">
                  <div className="flex items-center gap-2 text-xs text-slate-400 font-medium">
                    <Cpu className="w-4 h-4 text-emerald-500/80" />
                    <span>Zero-config deployment active.</span>
                  </div>
                  <button 
                    onClick={onUseUeransim}
                    className="text-xs text-emerald-400 font-semibold hover:text-emerald-300 transition flex items-center gap-1 group hover:underline"
                  >
                    Using UERANSIM instead?
                    <ArrowRight className="w-3.5 h-3.5 group-hover:translate-x-0.5 transition duration-150" />
                  </button>
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
                      Interface (NG Setup) established
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

      {/* Floating Developer Interactive Simulator Panel */}
      <div className="fixed bottom-6 left-1/2 -translate-x-1/2 bg-darkbg-900/80 backdrop-blur-md border border-darkbg-700/80 shadow-2xl px-5 py-3 rounded-full flex items-center gap-4 z-50 transition max-w-sm sm:max-w-xl overflow-x-auto whitespace-nowrap scrollbar-none">
        <div className="flex items-center gap-2 text-xs font-bold text-slate-400 border-r border-darkbg-800 pr-4 uppercase tracking-widest">
          <Sliders className="w-3.5 h-3.5 text-emerald-400 animate-pulse" />
          <span>Interactive Simulation</span>
        </div>
        
        <div className="flex items-center gap-2">
          {/* Waiting Trigger */}
          <button
            onClick={() => triggerSimulation("waiting")}
            className={`text-xs px-3 py-1.5 rounded-full font-mono font-medium transition ${
              isSimulated && simulatedState === "waiting" 
                ? "bg-slate-700 text-white border border-slate-600 shadow" 
                : "text-slate-400 hover:text-white"
            }`}
          >
            1. Waiting
          </button>

          {/* Failed Trigger */}
          <button
            onClick={() => triggerSimulation("failed")}
            className={`text-xs px-3 py-1.5 rounded-full font-mono font-medium transition ${
              isSimulated && simulatedState === "failed" 
                ? "bg-amber-500/20 text-amber-300 border border-amber-500/30 shadow" 
                : "text-slate-400 hover:text-white"
            }`}
          >
            2. Mismatch (Fail)
          </button>

          {/* Success Trigger */}
          <button
            onClick={() => triggerSimulation("connected")}
            className={`text-xs px-3 py-1.5 rounded-full font-mono font-medium transition ${
              isSimulated && simulatedState === "connected" 
                ? "bg-emerald-500/20 text-emerald-300 border border-emerald-500/30 shadow" 
                : "text-slate-400 hover:text-white"
            }`}
          >
            3. Connected
          </button>

          {/* Restore Live Stream */}
          {isSimulated && (
            <button
              onClick={resetToLive}
              className="text-[10px] bg-red-950/40 text-red-400 border border-red-900/30 px-2 py-1 rounded-md transition hover:bg-red-900/30"
            >
              Reset Live
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
