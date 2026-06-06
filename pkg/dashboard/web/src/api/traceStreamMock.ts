import type { QEvent, DiagnosticResult } from "./types";

const genId = () => Math.random().toString(36).substring(2, 9);

export const getMockEvents = (scenario: string, mode: "4g" | "5g", journeyId: string): QEvent[] => {
  const ts = (offsetSec: number) => new Date(Date.now() + offsetSec * 1000).toISOString();
  const is5G = mode === "5g";
  const protocolNas = is5G ? "nas5g" : "nas4g";
  const protocolRan = is5G ? "ngap" : "s1ap";
  const nfRan = is5G ? "gnb" : "enb";
  const nfAmf = is5G ? "amf" : "mme";
  const nfDb = is5G ? "udm" : "hss";
  const nfUplane = is5G ? "upf" : "spgw";

  const setupEvents: QEvent[] = [
    { id: genId(), journey_id: journeyId, timestamp: ts(0), nf: nfRan, category: "state_transition", severity: "info", protocol: protocolRan, message: `${nfRan.toUpperCase()} Association Connected`, payload: { gnb_assoc_id: `assoc-${journeyId}`, source_ip: "192.168.1.120" } },
    { id: genId(), journey_id: journeyId, timestamp: ts(0.5), nf: nfRan, category: "signaling_tx", severity: "info", protocol: protocolRan, message: `Sent ${is5G ? "NG" : "S1"} Setup Request`, payload: { enb_name: is5G ? "qcore-sim-5g" : "qcore-sim-4g", enb_id: 54321, plmn: "001/01", tac: 1, success: true } },
    { id: genId(), journey_id: journeyId, timestamp: ts(1.0), nf: nfRan, category: "signaling_rx", severity: "info", protocol: protocolRan, message: `Received ${is5G ? "NG" : "S1"} Setup Response`, payload: { success: true } }
  ];

  if (scenario === "wrong_mme_address") {
    return [{ id: genId(), journey_id: journeyId, timestamp: ts(0), nf: nfRan, category: "error", severity: "error", protocol: protocolRan, message: "SCTP Connection Timeout", payload: { code: "no_connection", message: `The simulated UE failed to connect to the ${is5G ? "AMF" : "MME"} address 127.0.0.1:1 because the bind port was refused or timed out.`, cause: "SCTP association timed out" } }];
  }

  const reqEvent: QEvent = { id: genId(), journey_id: journeyId, timestamp: ts(1.5), nf: protocolNas, category: "signaling_tx", severity: "info", protocol: protocolNas, message: is5G ? "Sent NAS Registration Request" : "Sent NAS Attach Request", payload: is5G ? { suci: "suci-0-001-01-0000-0-0-0000000001", reg_type: 1 } : { imsi: "001010000000001", attach_type: 1, tac: 1 } };

  if (scenario === "unprovisioned_imsi") {
    const rejectEvent: QEvent = { id: genId(), journey_id: journeyId, timestamp: ts(2.2), nf: nfDb, category: "error", severity: "error", protocol: "sbi", message: is5G ? "Registration Reject" : "Attach Reject", payload: { code: "unprovisioned_subscriber", message: "The subscriber IMSI 001010000000001 is not provisioned in the Core Network's subscriber database.", cause: is5G ? "Subscriber not found in UDR store" : "Subscriber not found in HSS database" } };
    return [...setupEvents, reqEvent, rejectEvent];
  }

  const authEvents: QEvent[] = [
    { id: genId(), journey_id: journeyId, timestamp: ts(2.0), nf: protocolNas, category: "signaling_rx", severity: "info", protocol: protocolNas, message: "Received NAS Authentication Request", payload: { supi: is5G ? "5G-SUPI-001010000000001" : "001010000000001", rand_prefix: "465b5ce8b199b49f" } },
    { id: genId(), journey_id: journeyId, timestamp: ts(2.5), nf: protocolNas, category: "signaling_tx", severity: "info", protocol: protocolNas, message: "Sent NAS Authentication Response", payload: { supi: is5G ? "5G-SUPI-001010000000001" : "001010000000001", success: true } }
  ];

  if (scenario === "wrong_ki") {
    const authRejectEvent: QEvent = { id: genId(), journey_id: journeyId, timestamp: ts(3.0), nf: is5G ? "ausf" : nfAmf, category: "error", severity: "error", protocol: is5G ? "sbi" : protocolNas, message: "Authentication Reject", payload: { code: "auth_failed", message: "RES mismatch (MAC verification failed) because the UE used a wrong Ki value.", cause: is5G ? "RES* does not match core expectation" : "RES does not match core expectation" } };
    return [...setupEvents, reqEvent, ...authEvents, authRejectEvent];
  }

  const securityEvents: QEvent[] = [
    { id: genId(), journey_id: journeyId, timestamp: ts(3.0), nf: protocolNas, category: "signaling_rx", severity: "info", protocol: protocolNas, message: "Received NAS Security Mode Command", payload: { supi: is5G ? "5G-SUPI-001010000000001" : "001010000000001", cipher_alg: 1, integ_alg: 1 } },
    { id: genId(), journey_id: journeyId, timestamp: ts(3.5), nf: protocolNas, category: "signaling_tx", severity: "info", protocol: protocolNas, message: "Sent NAS Security Mode Complete", payload: { supi: is5G ? "5G-SUPI-001010000000001" : "001010000000001" } },
    { id: genId(), journey_id: journeyId, timestamp: ts(4.0), nf: protocolRan, category: "signaling_rx", severity: "info", protocol: protocolRan, message: "Received Initial Context Setup Request" },
    { id: genId(), journey_id: journeyId, timestamp: ts(4.5), nf: protocolRan, category: "signaling_tx", severity: "info", message: "Sent Initial Context Setup Response" }
  ];

  const acceptEvents: QEvent[] = [
    { id: genId(), journey_id: journeyId, timestamp: ts(5.0), nf: protocolNas, category: "signaling_tx", severity: "info", protocol: protocolNas, message: is5G ? "Sent NAS Registration Complete" : "Sent NAS Attach Complete", payload: { supi: is5G ? "5G-SUPI-001010000000001" : "001010000000001", guti: is5G ? "5G-GUTI-001010000000001" : "GUTI-001010000000001" } }
  ];

  const pduRequestEvents: QEvent[] = is5G ? [
    { id: genId(), journey_id: journeyId, timestamp: ts(5.5), nf: protocolNas, category: "signaling_tx", severity: "info", protocol: protocolNas, message: "Sent NAS PDU Session Establishment Request", payload: { supi: "5G-SUPI-001010000000001", pdu_session_id: 1, dnn: "internet" } },
    { id: genId(), journey_id: journeyId, timestamp: ts(5.8), nf: "amf", category: "state_transition", severity: "info", protocol: "sbi", message: "Forwarding PDU Session Request to SMF", payload: { supi: "5G-SUPI-001010000000001", pdu_session_id: 1, dnn: "internet" } }
  ] : [
    { id: genId(), journey_id: journeyId, timestamp: ts(5.5), nf: "sgw", category: "state_transition", severity: "info", protocol: "s11", message: "SGW Create Session", payload: { imsi: "001010000000001", apn: "internet", ue_ip: "10.0.0.2", sgw_teid: 201 } }
  ];

  const sessionEstablishedEvents: QEvent[] = is5G ? [
    { id: genId(), journey_id: journeyId, timestamp: ts(6.2), nf: "smf", category: "state_transition", severity: "info", protocol: "sbi", message: "SMF Created PDU Session", payload: { supi: "5G-SUPI-001010000000001", pdu_session_id: 1, ue_ip: "10.45.0.2", upf_teid: 100 } },
    { id: genId(), journey_id: journeyId, timestamp: ts(6.5), nf: "upf", category: "state_transition", severity: "info", protocol: "gtp", message: "Received UPF Session Establishment", payload: { fseid: 501, upf_teid: 100 } },
    { id: genId(), journey_id: journeyId, timestamp: ts(6.8), nf: protocolNas, category: "signaling_rx", severity: "info", protocol: protocolNas, message: "Received NAS PDU Session Establishment Accept", payload: { supi: "5G-SUPI-001010000000001", pdu_session_id: 1, ue_ip: "10.45.0.2", upf_teid: 100 } }
  ] : [
    { id: genId(), journey_id: journeyId, timestamp: ts(6.0), nf: "sgw", category: "state_transition", severity: "info", protocol: "s11", message: "Modify Bearer Complete", payload: { imsi: "001010000000001", enb_addr: "192.168.1.101", enb_teid: 301 } }
  ];

  const dataFlowingEvents: QEvent[] = [
    { id: genId(), journey_id: journeyId, timestamp: ts(7.5), nf: nfUplane, category: "state_transition", severity: "info", protocol: "gtp", message: "Data Plane verification successful", payload: { supi: is5G ? "5G-SUPI-001010000000001" : "001010000000001", bytes_flowing: true, bitrate_kbps: 450 } }
  ];

  return [...setupEvents, reqEvent, ...authEvents, ...securityEvents, ...acceptEvents, ...pduRequestEvents, ...sessionEstablishedEvents, ...dataFlowingEvents];
};

export const getMockDiagnostic = (scenario: string): DiagnosticResult | null => {
  switch (scenario) {
    case "wrong_ki": return { Matched: true, Explanation: "The Core Network received a valid Authentication Response from the UE, but the calculated RES/RES* parameter did not match the credential vector stored in the database.", RootCause: "Authentication credential mismatch (RES mismatch).", Fix: "1. Update QCore's subscriber settings for IMSI 001010000000001 to use the correct secret key (Ki).\n2. If using a simulator, verify the scenario config contains matching Ki keys." };
    case "unprovisioned_imsi": return { Matched: true, Explanation: "The UE attempted an attach request using IMSI 001010000000001. The MME/AMF queried the subscriber registry (HSS/UDM) to retrieve credentials, but the database returned a null result.", RootCause: "Subscriber IMSI not found in database.", Fix: "1. Go to the 'Subscribers' tab in the navigation header.\n2. Click 'Add Subscriber' and register IMSI 001010000000001 with default values." };
    case "wrong_mme_address": return { Matched: true, Explanation: "The simulated UE attempted to initiate a connection, but the connection timed out.", RootCause: "SCTP association timed out (Refused/No connection).", Fix: "1. Check that MME/AMF bind port 38412 is correct and not blocked.\n2. Verify the configured address aligns with the local machine IP." };
    default: return null;
  }
};
