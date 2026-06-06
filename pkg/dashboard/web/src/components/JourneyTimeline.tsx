import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { 
  CheckCircle2, 
  XCircle, 
  ChevronRight, 
  ChevronDown, 
  Activity,
  Cpu,
  ShieldCheck,
  KeyRound,
  FileText,
  Network
} from "lucide-react";
import type { QEvent, DiagnosticResult } from "../api/types";

interface StepDetail {
  title: string;
  desc: string;
  longDesc: string;
  icon: React.ReactNode;
}

const JOURNEY_STEPS: StepDetail[] = [
  {
    title: "Registration Request",
    desc: "UE initiates registration using protected identity (SUCI/IMSI)",
    longDesc: "The User Equipment (UE) establishes a signaling connection and sends its mobile identity (SUCI in 5G, IMSI in 4G) to request network access.",
    icon: <Network className="w-5 h-5" />
  },
  {
    title: "Authentication",
    desc: "Core network performs mutual authentication with UE",
    longDesc: "The authentication center (UDM/HSS) generates vectors (using Milenage test vectors) to authenticate the UE, and the UE validates the network.",
    icon: <KeyRound className="w-5 h-5" />
  },
  {
    title: "Security Mode",
    desc: "Encryption and integrity protection algorithms negotiated",
    longDesc: "The core selects ciphering (e.g. NEA2) and integrity (e.g. NIA2) protection algorithms, securing all future NAS/NGAP signaling messages.",
    icon: <ShieldCheck className="w-5 h-5" />
  },
  {
    title: "Registration Accept",
    desc: "Core accepts request and allocates temporary identity (GUTI)",
    longDesc: "The Core Network registers the subscriber, allocates a temporary GUTI, and sends a complete registration confirmation to the UE.",
    icon: <CheckCircle2 className="w-5 h-5" />
  },
  {
    title: "PDU Session Request",
    desc: "UE requests data plane connectivity (APN/DNN session)",
    longDesc: "The registered UE requests a data session (PDU Session in 5G, PDN/EpsBearer in 4G) specifying the Data Network Name (DNN/APN).",
    icon: <FileText className="w-5 h-5" />
  },
  {
    title: "Session Established",
    desc: "User plane tunnel established and IP address allocated",
    longDesc: "The SMF/SGW allocates an IP address to the UE and establishes GTP-U tunnels with the UPF/SGW and gNB/eNB user planes.",
    icon: <Activity className="w-5 h-5" />
  },
  {
    title: "Data Flowing",
    desc: "User plane verified, active traffic flowing through UPF/SGW",
    longDesc: "User plane packets are successfully routed to/from the internet tun interface, confirming end-to-end user plane connectivity.",
    icon: <Cpu className="w-5 h-5" />
  }
];

interface JourneyTimelineProps {
  events: QEvent[];
  streaming: boolean;
  diagnostic: DiagnosticResult | null;
}

export default function JourneyTimeline({ events, streaming, diagnostic }: JourneyTimelineProps) {
  const [expandedStep, setExpandedStep] = useState<number | null>(null);

  // Map events to the 7 steps
  const processSteps = () => {
    // State of each of the 7 steps
    const stepStates = JOURNEY_STEPS.map((_, _index) => {
      return {
        status: "pending" as "pending" | "active" | "completed" | "failed",
        events: [] as QEvent[],
        payloads: {} as Record<string, any>,
        error: null as { message: string; cause?: string; fixGnb?: string; fixQCore?: string } | null
      };
    });

    // Check for errors anywhere in the trace
    const errorEvent = events.find(e => e.severity === "error");

    events.forEach(ev => {
      const msg = ev.message.toLowerCase();
      
      // Step 1: Registration Request
      if (
        msg.includes("setup request") || msg.includes("setuprequest") || 
        msg.includes("setup response") || msg.includes("setupresponse") ||
        msg.includes("registration request") || 
        msg.includes("attach request")
      ) {
        stepStates[0].events.push(ev);
        if (ev.payload) {
          stepStates[0].payloads = { ...stepStates[0].payloads, ...ev.payload };
        }
      }

      // Step 2: Authentication
      if (msg.includes("authentication") || msg.includes("auth reject")) {
        stepStates[1].events.push(ev);
        if (ev.payload) {
          stepStates[1].payloads = { ...stepStates[1].payloads, ...ev.payload };
        }
      }

      // Step 3: Security Mode
      if (
        msg.includes("security mode") || 
        msg.includes("context setup") ||
        msg.includes("contextsetup")
      ) {
        stepStates[2].events.push(ev);
        if (ev.payload) {
          stepStates[2].payloads = { ...stepStates[2].payloads, ...ev.payload };
        }
      }

      // Step 4: Registration Accept
      if (
        msg.includes("registration complete") || 
        msg.includes("attach complete") || 
        msg.includes("registration accept") || 
        msg.includes("attach accept") ||
        msg.includes("5gmm-registered")
      ) {
        stepStates[3].events.push(ev);
        if (ev.payload) {
          stepStates[3].payloads = { ...stepStates[3].payloads, ...ev.payload };
        }
      }

      // Step 5: PDU Session Request
      if (
        msg.includes("pdu session establishment request") || 
        msg.includes("forwarding pdu session") ||
        msg.includes("create session")
      ) {
        stepStates[4].events.push(ev);
        if (ev.payload) {
          stepStates[4].payloads = { ...stepStates[4].payloads, ...ev.payload };
        }
      }

      // Step 6: Session Established
      if (
        msg.includes("pdu session establishment accept") || 
        msg.includes("modify bearer complete") || 
        msg.includes("smf created pdu session") ||
        msg.includes("upf session establishment") ||
        msg.includes("pdu session created") ||
        msg.includes("n4 session establishment")
      ) {
        stepStates[5].events.push(ev);
        if (ev.payload) {
          stepStates[5].payloads = { ...stepStates[5].payloads, ...ev.payload };
        }
      }

      // Step 7: Data Flowing
      if (msg.includes("data plane") || msg.includes("data flowing")) {
        stepStates[6].events.push(ev);
        if (ev.payload) {
          stepStates[6].payloads = { ...stepStates[6].payloads, ...ev.payload };
        }
      }
    });

    // Mark completion states linearly
    // Step 1 Completed if Step 1 has events or subsequent steps have events
    const hasAnyEventAfter = (idx: number) => {
      for (let i = idx + 1; i < stepStates.length; i++) {
        if (stepStates[i].events.length > 0) return true;
      }
      return false;
    };

    // Define triggers for step completions
    const stepCompleted = (idx: number) => {
      if (hasAnyEventAfter(idx)) return true;
      const evs = stepStates[idx].events;
      if (evs.length === 0) return false;

      switch (idx) {
        case 0:
          return evs.some(e => e.message.toLowerCase().includes("setup response") || e.message.toLowerCase().includes("request"));
        case 1:
          return evs.some(e => e.message.toLowerCase().includes("response"));
        case 2:
          return evs.some(e => e.message.toLowerCase().includes("complete") || e.message.toLowerCase().includes("response"));
        case 3:
          return evs.some(e => e.message.toLowerCase().includes("complete"));
        case 4:
          return evs.some(e => e.message.toLowerCase().includes("request") || e.message.toLowerCase().includes("create"));
        case 5:
          return evs.some(e => e.message.toLowerCase().includes("accept") || e.message.toLowerCase().includes("modify bearer"));
        case 6:
          return evs.some(e => e.message.toLowerCase().includes("successful"));
        default:
          return false;
      }
    };

    // Compute status values
    let failureIndex = -1;

    for (let i = 0; i < stepStates.length; i++) {
      // If there is an error event in the trace, and it maps to this step (or this is where the trace ended)
      const stepHasError = stepStates[i].events.some(e => e.severity === "error");
      const isLastWithEvents = stepStates[i].events.length > 0 && !hasAnyEventAfter(i);

      if (errorEvent && (stepHasError || (isLastWithEvents && failureIndex === -1))) {
        stepStates[i].status = "failed";
        failureIndex = i;

        // Fill error details
        const errEv = stepStates[i].events.find(e => e.severity === "error") || errorEvent;
        stepStates[i].error = {
          message: errEv.message,
          cause: errEv.payload?.cause as string || errEv.payload?.message as string || "Unknown network error",
          fixGnb: errEv.payload?.fix_gnb_side as string || "Check configuration",
          fixQCore: errEv.payload?.fix_qcore_side as string || "Validate network settings"
        };
      } else if (failureIndex !== -1) {
        stepStates[i].status = "pending";
      } else if (stepCompleted(i)) {
        stepStates[i].status = "completed";
      } else if (i === 0 || stepCompleted(i - 1)) {
        stepStates[i].status = streaming ? "active" : "pending";
      } else {
        stepStates[i].status = "pending";
      }
    }

    return stepStates;
  };

  const steps = processSteps();

  const toggleExpand = (index: number) => {
    if (expandedStep === index) {
      setExpandedStep(null);
    } else {
      setExpandedStep(index);
    }
  };

  return (
    <div className="w-full max-w-2xl mx-auto space-y-6">
      <div className="text-xs font-bold text-slate-400 uppercase tracking-widest px-1">
        Signaling Journey (Chronological sequence)
      </div>

      <div className="relative border-l border-slate-800 ml-4 pl-8 space-y-8 py-2">
        {JOURNEY_STEPS.map((step, idx) => {
          const state = steps[idx];
          const isExpanded = expandedStep === idx;
          const status = state.status;

          // Compute style classes based on step status
          let borderClass = "border-slate-800";
          let bgClass = "bg-slate-900/20";
          let textClass = "text-slate-400";
          let iconWrapperClass = "bg-slate-950 border-slate-800 text-slate-600";
          let glowDot = null;

          if (status === "completed") {
            borderClass = "border-emerald-500/20";
            bgClass = "bg-emerald-500/5";
            textClass = "text-slate-200";
            iconWrapperClass = "bg-emerald-950 border-emerald-500/30 text-emerald-400";
          } else if (status === "active") {
            borderClass = "border-cyan-500/40 animate-glow";
            bgClass = "bg-cyan-500/5";
            textClass = "text-white";
            iconWrapperClass = "bg-cyan-950 border-cyan-500/50 text-cyan-400 animate-pulse";
            glowDot = (
              <span className="absolute -left-[37px] top-[14px] flex h-2.5 w-2.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-cyan-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-cyan-500"></span>
              </span>
            );
          } else if (status === "failed") {
            borderClass = "border-red-500/40 bg-red-950/10 shadow-lg shadow-red-950/20";
            bgClass = "bg-red-950/5";
            textClass = "text-red-200";
            iconWrapperClass = "bg-red-950 border-red-500/40 text-red-500";
          }

          return (
            <motion.div 
              key={idx} 
              layout="position"
              className={`relative card p-4 border transition-all duration-300 ${borderClass} ${bgClass} cursor-pointer group hover:border-slate-700`}
              onClick={() => toggleExpand(idx)}
            >
              {/* Stepper Dot on the Timeline line */}
              <div className={`absolute -left-[45px] top-[12px] w-[26px] h-[26px] rounded-full border flex items-center justify-center transition-all duration-300 ${iconWrapperClass}`}>
                {status === "completed" ? (
                  <CheckCircle2 className="w-4 h-4" />
                ) : status === "failed" ? (
                  <XCircle className="w-4 h-4" />
                ) : (
                  <span className="text-xs font-mono font-bold">{idx + 1}</span>
                )}
              </div>

              {glowDot}

              {/* Step Header */}
              <div className="flex items-start justify-between gap-4">
                <div className="space-y-1">
                  <h3 className={`text-base font-bold tracking-tight ${textClass} flex items-center gap-2`}>
                    {step.title}
                    {status === "active" && (
                      <span className="text-[10px] bg-cyan-500/10 text-cyan-400 font-mono px-2 py-0.5 rounded border border-cyan-500/20">
                        PROCESSING
                      </span>
                    )}
                  </h3>
                  <p className="text-xs text-slate-400 font-medium leading-relaxed">
                    {step.desc}
                  </p>
                </div>
                
                <div className="text-slate-500 group-hover:text-slate-300 transition duration-200 p-1">
                  {isExpanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                </div>
              </div>

              {/* Progressive Disclosure: Expanded Detail Panel */}
              <AnimatePresence>
                {isExpanded && (
                  <motion.div
                    initial={{ opacity: 0, height: 0 }}
                    animate={{ opacity: 1, height: "auto" }}
                    exit={{ opacity: 0, height: 0 }}
                    transition={{ duration: 0.25, ease: "easeInOut" }}
                    className="overflow-hidden mt-4 pt-4 border-t border-slate-800 space-y-4"
                    onClick={(e) => e.stopPropagation()} // prevent collapsing on click inside
                  >
                    <p className="text-xs text-slate-400 leading-normal font-sans">
                      {step.longDesc}
                    </p>

                    {/* Step telemetry fields if completed */}
                    {Object.keys(state.payloads).length > 0 && (
                      <div className="bg-slate-950/60 rounded-xl p-3.5 border border-slate-850 space-y-2">
                        <div className="text-[10px] font-bold text-slate-500 uppercase tracking-widest border-b border-slate-900 pb-1.5 flex items-center gap-1.5">
                          {step.icon}
                          <span>Decoded Parameters</span>
                        </div>
                        <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-xs font-mono">
                          {Object.entries(state.payloads).map(([key, val]) => {
                            // Don't show technical internal objects
                            if (typeof val === "object" && val !== null) return null;
                            // Format keys humanely
                            const keyLabel = key.replace(/_/g, " ").replace(/\b\w/g, c => c.toUpperCase());
                            return (
                              <div key={key} className="flex justify-between border-b border-slate-900/30 py-1">
                                <span className="text-slate-500">{keyLabel}</span>
                                <span className="text-slate-300 font-semibold truncate max-w-[200px]" title={String(val)}>
                                  {String(val)}
                                </span>
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    )}

                    {/* Stage failure details and fixes (inline red card) */}
                    {status === "failed" && state.error && (
                      <div className="bg-red-500/5 border border-red-500/20 rounded-xl p-4 space-y-3 animate-glow">
                        <div className="flex items-center gap-2 text-red-400">
                          <XCircle className="w-5 h-5" />
                          <span className="text-xs font-bold uppercase tracking-wider">JOURNEY HALTED</span>
                        </div>
                        <p className="text-xs text-red-200 leading-relaxed bg-red-950/20 p-3 rounded-lg border border-red-500/10 font-mono">
                          {state.error.message}
                        </p>
                        
                        {/* Cause & Fix Panel */}
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                          <div className="bg-slate-950/40 p-3 rounded-lg border border-slate-900">
                            <span className="text-[10px] font-bold text-red-400 uppercase tracking-wider">Fix on gNB/Device</span>
                            <p className="text-slate-300 mt-1 font-mono">{state.error.fixGnb}</p>
                          </div>
                          <div className="bg-slate-950/40 p-3 rounded-lg border border-slate-900">
                            <span className="text-[10px] font-bold text-emerald-400 uppercase tracking-wider">Fix on QCore side</span>
                            <p className="text-slate-300 mt-1 font-mono">{state.error.fixQCore}</p>
                          </div>
                        </div>
                      </div>
                    )}

                    {/* List of raw events mapped to this step */}
                    {state.events.length > 0 && (
                      <div className="space-y-1.5">
                        <div className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">
                          Decoded Signaling Frames
                        </div>
                        <div className="space-y-1.5">
                          {state.events.map((ev) => {
                            const isErr = ev.severity === "error";
                            return (
                              <div key={ev.id} className="bg-slate-950/40 border border-slate-900/60 p-2.5 rounded-lg flex items-center justify-between text-xs font-mono">
                                <div className="flex items-center gap-2">
                                  <span className={`text-[9px] uppercase font-bold px-1.5 py-0.5 rounded ${
                                    isErr 
                                      ? "bg-red-500/10 text-red-400 border border-red-500/20"
                                      : "bg-slate-800 text-slate-400"
                                  }`}>
                                    {ev.nf}
                                  </span>
                                  <span className="text-slate-300">{ev.message}</span>
                                </div>
                                <span className="text-[10px] text-slate-500">
                                  {new Date(ev.timestamp).toLocaleTimeString([], { hour12: false })}
                                </span>
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    )}
                  </motion.div>
                )}
              </AnimatePresence>
            </motion.div>
          );
        })}
      </div>

      {/* AI Diagnostic Summary when Attach ends in error */}
      {diagnostic && (
        <motion.div 
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          className="card border-indigo-500/20 bg-indigo-500/5 space-y-4 p-5 animate-glow relative overflow-hidden"
        >
          {/* Accent radial glow */}
          <div className="absolute top-0 right-0 w-48 h-48 bg-indigo-500/5 rounded-full blur-3xl pointer-events-none" />

          <div className="flex items-center gap-2">
            <span className="text-lg">✨</span>
            <h4 className="text-sm font-bold text-white uppercase tracking-wider flex items-center gap-2">
              Diagnostic AI Report
              <span className="text-[9px] bg-indigo-500/15 text-indigo-400 font-bold px-2 py-0.5 rounded border border-indigo-500/20">
                ACTIVE
              </span>
            </h4>
          </div>
          <div className="space-y-4 text-xs">
            <div>
              <span className="font-bold text-slate-400 uppercase tracking-widest block mb-1">What Happened</span>
              <p className="text-slate-300 leading-relaxed">{diagnostic.Explanation}</p>
            </div>
            <div>
              <span className="font-bold text-slate-400 uppercase tracking-widest block mb-1">Root Cause</span>
              <p className="text-slate-300 leading-relaxed font-mono">{diagnostic.RootCause}</p>
            </div>
            <div className="bg-slate-950/60 p-4 rounded-xl border border-slate-850">
              <span className="font-bold text-indigo-400 uppercase tracking-widest block mb-1">Suggested Fix</span>
              <p className="text-slate-200 leading-relaxed font-mono">{diagnostic.Fix}</p>
            </div>
          </div>
        </motion.div>
      )}
    </div>
  );
}
