// Package diag provides deterministic, rules-based diagnostic logic for QCore
// network-function procedures. Rules produce structured causes and fixes for both
// the operator (gNB/device side) and the QCore side — no model inference, no
// guessing. Wrong output here is a confident liar; every rule must be precise.
//
// This is the substrate that feeds the hero screen (docs/ui-ux-design.md §2) and
// the Phase C AI catalog (pkg/ai). The AI reasons *over* these results; it does not
// replace them.
package diag

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/qcore-project/qcore/pkg/ident"
	"github.com/qcore-project/qcore/pkg/ngap"
)

// NGSetupResult is the output of DiagnoseNGSetup. When OK is true the setup
// succeeded and Cause/Fix fields are empty. When OK is false the operator sees
// exactly what mismatched and what to change on each side.
type NGSetupResult struct {
	OK bool

	// Failure fields — populated only when OK == false.
	Cause        string // machine-readable tag: "plmn_mismatch", "tac_mismatch", …
	Explanation  string // plain English: what happened
	FixGNBSide   string // what the gNB operator must change
	FixQCoreSide string // what the QCore operator can change as an alternative
}

// AMFConfig is the subset of AMF configuration DiagnoseNGSetup needs to compare
// against what the gNB advertised. Extracted as a plain struct so pkg/diag has
// no import cycle with pkg/amf.
type AMFConfig struct {
	// SupportedPLMNs is the list of PLMN identities this AMF serves.
	// Each entry is a 3-byte BCD value encoded by pkg/ident.
	SupportedPLMNs []ngap.PLMN

	// SupportedTACs is the set of TAC values (3-byte) the AMF considers valid.
	// An empty slice means "any TAC is accepted."
	SupportedTACs [][3]byte

	// SupportedSNSSAIs is the set of slices the AMF offers. An empty slice means
	// "any SST is accepted." Only SST is compared; SD is treated as a wildcard.
	SupportedSNSSAIs []ngap.SNSSAI
}

// DiagnoseNGSetup evaluates a decoded NGSetupRequest against the AMF's
// configured parameters and returns a structured result. It checks — in order:
//  1. PLMN mismatch (gNB PLMN not in AMF's supported list)
//  2. TAC mismatch (none of the gNB's TACs match the AMF's list)
//  3. S-NSSAI/slice mismatch (gNB offered no slice the AMF supports)
//
// A "no connection attempt" failure is detected upstream by the caller (it
// means the event stream has no GNBConnected event after a timeout); this
// function is not called in that case.
//
// Malformed NG Setup Request errors are surfaced by the caller as a non-nil
// DecodeNGSetupRequest error before DiagnoseNGSetup is called.
func DiagnoseNGSetup(req *ngap.NGSetupRequest, cfg AMFConfig) NGSetupResult {
	// --- 1. PLMN check --------------------------------------------------------
	// The gNB advertises its PLMN in GlobalRANNodeID. Check each PLMN the gNB
	// broadcasts (via SupportedTAList → BroadcastPLMN) against the AMF list.
	// The GlobalRANNodeID PLMN is the authoritative identity; the broadcast PLMNs
	// in SupportedTA are per-cell and may differ (rare). We check the GNB-ID PLMN
	// first, then scan broadcast PLMNs.
	gnbPLMN := req.GlobalRANNodeID.PLMN
	gnbMCC, gnbMNC := ident.DecodePLMN([3]byte(gnbPLMN))

	if len(cfg.SupportedPLMNs) > 0 && !plmnInList(gnbPLMN, cfg.SupportedPLMNs) {
		// Build a readable list of the AMF's supported PLMNs for the fix message.
		amfPLMNs := plmnListHuman(cfg.SupportedPLMNs)
		return NGSetupResult{
			Cause: "plmn_mismatch",
			Explanation: fmt.Sprintf(
				"The gNB advertised PLMN %s/%s (MCC=%s, MNC=%s). "+
					"QCore is configured for: %s. "+
					"The PLMN must match exactly before NG Setup can succeed.",
				gnbMCC, gnbMNC, gnbMCC, gnbMNC, amfPLMNs,
			),
			FixGNBSide: fmt.Sprintf(
				"Set the gNB's MCC and MNC to one of QCore's supported PLMNs: %s. "+
					"In UERANSIM: edit mcc/mnc in gnb.yaml. "+
					"In a real gNB: update the PLMN ID in the RAN configuration.",
				amfPLMNs,
			),
			FixQCoreSide: fmt.Sprintf(
				"Add PLMN %s/%s to QCore's AMF configured PLMN list (amf.plmn in config.yaml). "+
					"Restart the AMF for the change to take effect.",
				gnbMCC, gnbMNC,
			),
		}
	}

	// --- 2. TAC check ---------------------------------------------------------
	// Collect all TACs the gNB advertised. If the AMF has a non-empty TAC list,
	// at least one TAC must match.
	if len(cfg.SupportedTACs) > 0 {
		var gnbTACs [][3]byte
		for _, ta := range req.SupportedTAList {
			gnbTACs = append(gnbTACs, ta.TAC)
		}
		if !tacListOverlaps(gnbTACs, cfg.SupportedTACs) {
			gnbTACsHex := tacListHex(gnbTACs)
			amfTACsHex := tacListHex(cfg.SupportedTACs)
			return NGSetupResult{
				Cause: "tac_mismatch",
				Explanation: fmt.Sprintf(
					"The gNB advertised TAC(s) %s but QCore is configured for TAC(s) %s. "+
						"The Tracking Area Code must overlap for the gNB to register.",
					gnbTACsHex, amfTACsHex,
				),
				FixGNBSide: fmt.Sprintf(
					"Set the gNB's TAC to one of QCore's supported values: %s. "+
						"In UERANSIM: edit tac in gnb.yaml. "+
						"In a real gNB: update the Tracking Area Code in cell configuration.",
					amfTACsHex,
				),
				FixQCoreSide: fmt.Sprintf(
					"Add TAC %s to QCore's AMF configuration (amf.tac in config.yaml). "+
						"Restart the AMF for the change to take effect.",
					gnbTACsHex,
				),
			}
		}
	}

	// --- 3. S-NSSAI / slice check ---------------------------------------------
	// Collect all SSTs the gNB broadcast. If the AMF has a non-empty SNSSAI list,
	// at least one SST must match.
	if len(cfg.SupportedSNSSAIs) > 0 {
		var gnbSSTs []uint8
		for _, ta := range req.SupportedTAList {
			for _, bp := range ta.BroadcastPLMN {
				for _, s := range bp.SNSSAI {
					gnbSSTs = appendIfAbsent(gnbSSTs, s.SST)
				}
			}
		}
		if !ssstListOverlaps(gnbSSTs, cfg.SupportedSNSSAIs) {
			amfSlices := snssaiListHuman(cfg.SupportedSNSSAIs)
			gnbSlices := sstListHuman(gnbSSTs)
			return NGSetupResult{
				Cause: "slice_mismatch",
				Explanation: fmt.Sprintf(
					"The gNB offered S-NSSAI SST(s) %s but QCore supports %s. "+
						"At least one slice must be in common for NG Setup to succeed.",
					gnbSlices, amfSlices,
				),
				FixGNBSide: fmt.Sprintf(
					"Configure the gNB to broadcast one of QCore's supported slices: %s. "+
						"In UERANSIM: add the SST value to slices in gnb.yaml. "+
						"In a real gNB: update the configured S-NSSAIs.",
					amfSlices,
				),
				FixQCoreSide: fmt.Sprintf(
					"Add SST %s to QCore's PLMNSupportList in the AMF configuration. "+
						"Restart the AMF for the change to take effect.",
					gnbSlices,
				),
			}
		}
	}

	return NGSetupResult{OK: true}
}

// DiagnoseNGSetupMalformed returns a structured result for an NG Setup where
// NGAP decoding itself failed (malformed PDU).
func DiagnoseNGSetupMalformed(decodeErr error) NGSetupResult {
	return NGSetupResult{
		Cause: "malformed_request",
		Explanation: fmt.Sprintf(
			"QCore could not decode the NG Setup Request: %v. "+
				"The gNB sent an NGAP PDU that does not conform to TS 38.413.",
			decodeErr,
		),
		FixGNBSide: "Check the gNB's NGAP library version and NG Setup Request encoding. " +
			"Compare the raw NGAP bytes against TS 38.413 §9.2.6.1. " +
			"If using UERANSIM, ensure you are running a supported version.",
		FixQCoreSide: "If the gNB is known-good, file an issue — QCore may have a decoder bug. " +
			"Include the raw hex NGAP PDU bytes captured at the AMF NGAP port.",
	}
}

// DiagnoseNoConnection returns the result for the case where no NG Setup
// attempt arrived within the expected window (caller decides the timeout).
func DiagnoseNoConnection(amfAddr, amfPLMN string) NGSetupResult {
	return NGSetupResult{
		Cause: "no_connection",
		Explanation: fmt.Sprintf(
			"QCore's AMF has been listening on %s but no gNB has attempted NG Setup. "+
				"The most common causes are a wrong AMF address on the gNB, or a "+
				"firewall blocking SCTP/TCP port 38412.",
			amfAddr,
		),
		FixGNBSide: fmt.Sprintf(
			"Verify the gNB's AMF address is set to %s and port 38412. "+
				"In UERANSIM: check amfConfigs[].address in gnb.yaml. "+
				"In a real gNB: verify the S1AP/NGAP AMF IP in the cell configuration. "+
				"Also check that no firewall or NAT is blocking SCTP/TCP port 38412.",
			amfAddr,
		),
		FixQCoreSide: "Confirm the AMF is listening: the QCore dashboard shows 'Waiting for gNB' "+
			"with the correct bind address. If the address shown is 0.0.0.0, "+
			"find the host's actual IP and provide that to the gNB operator. "+
			"PLMN configured: " + amfPLMN + ".",
	}
}

// --- helpers ------------------------------------------------------------------

func plmnInList(p ngap.PLMN, list []ngap.PLMN) bool {
	for _, l := range list {
		if l == p {
			return true
		}
	}
	return false
}

func plmnListHuman(list []ngap.PLMN) string {
	parts := make([]string, len(list))
	for i, p := range list {
		mcc, mnc := ident.DecodePLMN([3]byte(p))
		parts[i] = mcc + "/" + mnc
	}
	return strings.Join(parts, ", ")
}

func tacListOverlaps(a, b [][3]byte) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func tacListHex(list [][3]byte) string {
	parts := make([]string, len(list))
	for i, t := range list {
		parts[i] = strings.ToUpper(hex.EncodeToString(t[:]))
	}
	return strings.Join(parts, ", ")
}

func ssstListOverlaps(gnbSSTs []uint8, amf []ngap.SNSSAI) bool {
	for _, gnb := range gnbSSTs {
		for _, a := range amf {
			if a.SST == gnb {
				return true
			}
		}
	}
	return false
}

func snssaiListHuman(list []ngap.SNSSAI) string {
	parts := make([]string, len(list))
	for i, s := range list {
		parts[i] = fmt.Sprintf("SST=%d", s.SST)
	}
	return strings.Join(parts, ", ")
}

func sstListHuman(ssts []uint8) string {
	parts := make([]string, len(ssts))
	for i, s := range ssts {
		parts[i] = fmt.Sprintf("SST=%d", s)
	}
	return strings.Join(parts, ", ")
}

func appendIfAbsent(s []uint8, v uint8) []uint8 {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
