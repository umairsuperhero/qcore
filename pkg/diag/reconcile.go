package diag

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/qcore-project/qcore/pkg/subscriber"
	"gopkg.in/yaml.v3"
)

const (
	CauseServingNetworkMismatch = "serving_network_mismatch"
	CauseDNNMismatch            = "dnn_mismatch"
	CauseKiMismatch             = "ki_mismatch"
	CauseOPcMismatch            = "opc_mismatch"
)

var hex32Pattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// ReconcileInput is the normalized RAN/UE config surface used for static,
// pre-attach comparison. It is intentionally protocol-small and maps to the
// fields QCore already advertises in the RAN config panel.
type ReconcileInput struct {
	RANPLMN              string   `json:"ran_plmn,omitempty"`
	UEPLMN               string   `json:"ue_plmn,omitempty"`
	TAC                  *uint16  `json:"tac,omitempty"`
	SNSSAIs              []SNSSAI `json:"snssais,omitempty"`
	ServingNetworkName   string   `json:"serving_network_name,omitempty"`
	IMSI                 string   `json:"imsi,omitempty"`
	Ki                   string   `json:"ki,omitempty"`
	OP                   string   `json:"op,omitempty"`
	OPc                  string   `json:"opc,omitempty"`
	OPType               string   `json:"op_type,omitempty"`
	SUCIProtectionScheme string   `json:"suci_protection_scheme,omitempty"`
	DNN                  string   `json:"dnn,omitempty"`
}

type SNSSAI struct {
	SST uint8  `json:"sst"`
	SD  string `json:"sd,omitempty"`
}

type ReconcileCore struct {
	PLMN               string
	TAC                uint16
	SNSSAIs            []SNSSAI
	ServingNetworkName string
	DefaultDNN         string
	Subscriber         *SubscriberCredentials
}

type SubscriberCredentials struct {
	IMSI string
	Ki   string
	OPc  string
	APN  string
}

type ReconcileResult struct {
	OK         bool              `json:"ok"`
	Summary    string            `json:"summary"`
	Mismatches []ConfigMismatch  `json:"mismatches"`
	Normalized ReconcileInput    `json:"normalized"`
	Core       map[string]string `json:"core,omitempty"`
}

type ConfigMismatch struct {
	Field     string `json:"field"`
	RANValue  string `json:"ran_value"`
	CoreValue string `json:"core_value"`
	Cause     string `json:"cause"`
	Fix       string `json:"fix"`
	Severity  string `json:"severity"`
}

func ParseReconcileInput(data []byte) (ReconcileInput, error) {
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return ReconcileInput{}, fmt.Errorf("empty config body")
	}

	root, err := parseYAMLMap(raw)
	if err != nil {
		return ReconcileInput{}, err
	}

	var out ReconcileInput

	// Preferred API shape: {"gnb_yaml":"...", "ue_yaml":"..."}.
	if gnbText := firstString(root, "gnb_yaml", "gnbYaml", "gnb"); gnbText != "" {
		gnb, err := parseYAMLMap(gnbText)
		if err != nil {
			return ReconcileInput{}, fmt.Errorf("parsing gNB config: %w", err)
		}
		applyGNBFields(&out, gnb)
	}
	if ueText := firstString(root, "ue_yaml", "ueYaml", "ue"); ueText != "" {
		ue, err := parseYAMLMap(ueText)
		if err != nil {
			return ReconcileInput{}, fmt.Errorf("parsing UE config: %w", err)
		}
		applyUEFields(&out, ue)
	}

	// Also support nested JSON objects and flat JSON/YAML fields.
	if gnbMap := firstMap(root, "gnb", "gnb_config", "gnbConfig"); gnbMap != nil {
		applyGNBFields(&out, gnbMap)
	}
	if ueMap := firstMap(root, "ue", "ue_config", "ueConfig"); ueMap != nil {
		applyUEFields(&out, ueMap)
	}
	applyGNBFields(&out, root)
	applyUEFields(&out, root)

	return out, nil
}

func ReconcileConfig(in ReconcileInput, core ReconcileCore) ReconcileResult {
	if core.DefaultDNN == "" {
		core.DefaultDNN = "internet"
	}
	if len(core.SNSSAIs) == 0 {
		core.SNSSAIs = []SNSSAI{{SST: 1, SD: "000001"}}
	}

	var mismatches []ConfigMismatch
	add := func(field, ranValue, coreValue, cause, fix, severity string) {
		mismatches = append(mismatches, ConfigMismatch{
			Field:     field,
			RANValue:  ranValue,
			CoreValue: coreValue,
			Cause:     cause,
			Fix:       fix,
			Severity:  severity,
		})
	}

	seenPLMN := map[string]bool{}
	for _, candidate := range []struct {
		field string
		plmn  string
	}{
		{"gnb.plmn", normalizePLMN(in.RANPLMN)},
		{"ue.plmn", normalizePLMN(in.UEPLMN)},
	} {
		if candidate.plmn == "" || seenPLMN[candidate.field+"="+candidate.plmn] {
			continue
		}
		seenPLMN[candidate.field+"="+candidate.plmn] = true
		if candidate.plmn != normalizePLMN(core.PLMN) {
			add(candidate.field, humanPLMN(candidate.plmn), humanPLMN(core.PLMN), "plmn_mismatch",
				"Set MCC/MNC in the RAN and UE config to "+humanPLMN(core.PLMN)+" or change QCore's AMF PLMN intentionally.", "error")
		}
	}

	if in.TAC != nil && *in.TAC != core.TAC {
		add("gnb.tac", fmt.Sprintf("%d", *in.TAC), fmt.Sprintf("%d", core.TAC), "tac_mismatch",
			fmt.Sprintf("Set tac in gnb.yaml to %d, or intentionally change amf.tac in QCore and restart the AMF.", core.TAC), "error")
	}

	if len(in.SNSSAIs) > 0 && !snssaiOverlaps(in.SNSSAIs, core.SNSSAIs) {
		add("s-nssai", snssaiList(in.SNSSAIs), snssaiList(core.SNSSAIs), "slice_mismatch",
			"Set the RAN/UE configured NSSAI to "+snssaiList(core.SNSSAIs)+". For UERANSIM, update slices, configured-nssai, default-nssai, and session.slice.", "error")
	}

	if in.ServingNetworkName != "" && core.ServingNetworkName != "" &&
		!strings.EqualFold(strings.TrimSpace(in.ServingNetworkName), strings.TrimSpace(core.ServingNetworkName)) {
		add("serving_network_name", in.ServingNetworkName, core.ServingNetworkName, CauseServingNetworkMismatch,
			"Set the UE serving network name to "+core.ServingNetworkName+" so 5G-AKA derives the same keys on both sides.", "error")
	}

	sub := core.Subscriber
	if in.IMSI != "" && sub == nil {
		add("ue.imsi", in.IMSI, "not provisioned", CauseUnknownSubscriber,
			"Add subscriber "+in.IMSI+" in QCore with the UE's Ki, OPc, AMF, and DNN before attaching.", "error")
	}
	if sub != nil {
		if in.IMSI != "" && in.IMSI != sub.IMSI {
			add("ue.imsi", in.IMSI, sub.IMSI, CauseUnknownSubscriber,
				"Use IMSI "+sub.IMSI+" in ue.yaml, or provision "+in.IMSI+" in QCore first.", "error")
		}
		if in.Ki != "" && !sameHexSecret(in.Ki, sub.Ki) {
			add("ue.key", secretLabel(in.Ki), secretLabel(sub.Ki), CauseKiMismatch,
				"Set the UE key/Ki byte-for-byte to the Ki stored for subscriber "+sub.IMSI+" in QCore.", "error")
		}

		opc, opcSource := effectiveOPc(in)
		if opcSource != "" && !sameHexSecret(opc, sub.OPc) {
			add(opcSource, secretLabel(opc), secretLabel(sub.OPc), CauseOPcMismatch,
				"Set UE OPc to the OPc stored for subscriber "+sub.IMSI+". If the SIM profile uses OP, derive OPc = AES_Ki(OP) XOR OP with the same Ki.", "error")
		}

		dnn := strings.TrimSpace(in.DNN)
		coreDNN := sub.APN
		if coreDNN == "" {
			coreDNN = core.DefaultDNN
		}
		if dnn != "" && !strings.EqualFold(dnn, coreDNN) {
			add("ue.dnn", dnn, coreDNN, CauseDNNMismatch,
				"Set the UE session APN/DNN to "+coreDNN+" or provision the subscriber/session for "+dnn+" intentionally.", "error")
		}
	} else if in.DNN != "" && !strings.EqualFold(in.DNN, core.DefaultDNN) {
		add("ue.dnn", in.DNN, core.DefaultDNN, CauseDNNMismatch,
			"Set the UE session APN/DNN to "+core.DefaultDNN+", which is QCore's default served DNN.", "warn")
	}

	if scheme := normalizeScheme(in.SUCIProtectionScheme); scheme != "" && scheme != "0" && scheme != "null" && scheme != "1" && scheme != "2" {
		add("ue.suci_protection_scheme", in.SUCIProtectionScheme, "0/null-scheme, 1/Profile A, or 2/Profile B", CauseSUCIDecodeFailure,
			"Set UERANSIM protectionScheme to 0, 1, or 2. For Profile A/B, make sure homeNetworkPublicKeyId matches a configured UDM sidf_keys entry.", "error")
	}

	if len(mismatches) == 0 {
		return ReconcileResult{
			OK:         true,
			Summary:    "RAN and UE config match QCore for the checked fields.",
			Normalized: in,
			Core:       coreSummary(core),
		}
	}
	return ReconcileResult{
		OK:         false,
		Summary:    fmt.Sprintf("%d config mismatch(es) found before attach.", len(mismatches)),
		Mismatches: mismatches,
		Normalized: in,
		Core:       coreSummary(core),
	}
}

func parseYAMLMap(raw string) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("invalid YAML/JSON: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("config body must be an object")
	}
	return m, nil
}

func applyGNBFields(out *ReconcileInput, m map[string]interface{}) {
	if plmn := plmnFromMap(m); plmn != "" && out.RANPLMN == "" {
		out.RANPLMN = plmn
	}
	if tac, ok := uint16FromAny(firstValue(m, "tac", "TAC")); ok && out.TAC == nil {
		out.TAC = &tac
	}
	if snssais := slicesFromAny(firstValue(m, "slices", "snssai", "s_nssai")); len(snssais) > 0 {
		out.SNSSAIs = appendMissingSNSSAI(out.SNSSAIs, snssais...)
	}
	if snn := firstString(m, "servingNetworkName", "serving_network_name", "snn"); snn != "" && out.ServingNetworkName == "" {
		out.ServingNetworkName = snn
	}
}

func applyUEFields(out *ReconcileInput, m map[string]interface{}) {
	if plmn := plmnFromMap(m); plmn != "" && out.UEPLMN == "" {
		out.UEPLMN = plmn
	}
	if imsi := imsiFromMap(m); imsi != "" && out.IMSI == "" {
		out.IMSI = imsi
	}
	if ki := firstString(m, "key", "ki", "Ki"); ki != "" && out.Ki == "" {
		out.Ki = strings.ToLower(strings.TrimSpace(ki))
	}
	if opc := firstString(m, "opc", "OPc", "OPC"); opc != "" && out.OPc == "" {
		out.OPc = strings.ToLower(strings.TrimSpace(opc))
	}
	if op := firstString(m, "op", "OP"); op != "" && out.OP == "" {
		out.OP = strings.ToLower(strings.TrimSpace(op))
	}
	if opType := firstString(m, "opType", "op_type", "optype"); opType != "" && out.OPType == "" {
		out.OPType = strings.ToUpper(strings.TrimSpace(opType))
	}
	if strings.EqualFold(out.OPType, "OPC") && out.OPc == "" && out.OP != "" {
		out.OPc = out.OP
	}
	if scheme := firstString(m, "protectionScheme", "protection_scheme", "suciProtectionScheme", "suci_protection_scheme"); scheme != "" && out.SUCIProtectionScheme == "" {
		out.SUCIProtectionScheme = strings.TrimSpace(scheme)
	}
	if snn := firstString(m, "servingNetworkName", "serving_network_name", "snn"); snn != "" && out.ServingNetworkName == "" {
		out.ServingNetworkName = snn
	}

	for _, key := range []string{"configured-nssai", "configuredNssai", "default-nssai", "defaultNssai", "requested-nssai", "requestedNssai"} {
		if snssais := slicesFromAny(m[key]); len(snssais) > 0 {
			out.SNSSAIs = appendMissingSNSSAI(out.SNSSAIs, snssais...)
		}
	}
	if snssais := slicesFromAny(firstValue(m, "snssai", "s_nssai", "slice")); len(snssais) > 0 {
		out.SNSSAIs = appendMissingSNSSAI(out.SNSSAIs, snssais...)
	}

	if dnn := firstString(m, "dnn", "apn", "defaultDnn", "default_dnn"); dnn != "" && out.DNN == "" {
		out.DNN = strings.TrimSpace(dnn)
	}
	if sessions, ok := firstValue(m, "sessions").([]interface{}); ok {
		for _, item := range sessions {
			sm, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if out.DNN == "" {
				out.DNN = firstString(sm, "apn", "dnn")
			}
			if snssais := slicesFromAny(firstValue(sm, "slice", "snssai", "s_nssai")); len(snssais) > 0 {
				out.SNSSAIs = appendMissingSNSSAI(out.SNSSAIs, snssais...)
			}
		}
	}
}

func plmnFromMap(m map[string]interface{}) string {
	if plmn := firstString(m, "plmn", "PLMN"); plmn != "" {
		return normalizePLMN(plmn)
	}
	mcc := firstString(m, "mcc", "MCC")
	mnc := firstString(m, "mnc", "MNC")
	if mcc == "" || mnc == "" {
		return ""
	}
	return normalizePLMN(mcc + mnc)
}

func imsiFromMap(m map[string]interface{}) string {
	for _, key := range []string{"imsi", "IMSI"} {
		if v := firstString(m, key); v != "" {
			return strings.TrimPrefix(strings.TrimSpace(v), "imsi-")
		}
	}
	if supi := firstString(m, "supi", "SUPI"); strings.HasPrefix(supi, "imsi-") {
		return strings.TrimPrefix(supi, "imsi-")
	}
	return ""
}

func slicesFromAny(v interface{}) []SNSSAI {
	switch x := v.(type) {
	case []interface{}:
		var out []SNSSAI
		for _, item := range x {
			out = append(out, slicesFromAny(item)...)
		}
		return out
	case map[string]interface{}:
		sst64, ok := uint64FromAny(firstValue(x, "sst", "SST"))
		if !ok || sst64 == 0 || sst64 > 255 {
			return nil
		}
		return []SNSSAI{{SST: uint8(sst64), SD: normalizeSD(firstString(x, "sd", "SD"))}}
	default:
		return nil
	}
}

func appendMissingSNSSAI(base []SNSSAI, add ...SNSSAI) []SNSSAI {
	for _, s := range add {
		found := false
		for _, existing := range base {
			if existing.SST == s.SST && strings.EqualFold(existing.SD, s.SD) {
				found = true
				break
			}
		}
		if !found {
			base = append(base, s)
		}
	}
	return base
}

func snssaiOverlaps(ran, core []SNSSAI) bool {
	for _, r := range ran {
		for _, c := range core {
			if r.SST != c.SST {
				continue
			}
			if c.SD == "" || r.SD == "" || strings.EqualFold(r.SD, c.SD) {
				return true
			}
		}
	}
	return false
}

func snssaiList(list []SNSSAI) string {
	parts := make([]string, 0, len(list))
	for _, s := range list {
		if s.SD != "" {
			parts = append(parts, fmt.Sprintf("SST=%d SD=%s", s.SST, s.SD))
		} else {
			parts = append(parts, fmt.Sprintf("SST=%d", s.SST))
		}
	}
	return strings.Join(parts, ", ")
}

func effectiveOPc(in ReconcileInput) (string, string) {
	if in.OPc != "" {
		return strings.ToLower(strings.TrimSpace(in.OPc)), "ue.opc"
	}
	if strings.EqualFold(in.OPType, "OPC") && in.OP != "" {
		return strings.ToLower(strings.TrimSpace(in.OP)), "ue.op"
	}
	if strings.EqualFold(in.OPType, "OP") && in.OP != "" && in.Ki != "" {
		kiBytes, err := hex.DecodeString(strings.TrimSpace(in.Ki))
		if err != nil || len(kiBytes) != 16 {
			return strings.ToLower(strings.TrimSpace(in.OP)), "ue.op"
		}
		opBytes, err := hex.DecodeString(strings.TrimSpace(in.OP))
		if err != nil || len(opBytes) != 16 {
			return strings.ToLower(strings.TrimSpace(in.OP)), "ue.op"
		}
		var ki, op [16]byte
		copy(ki[:], kiBytes)
		copy(op[:], opBytes)
		opc, err := subscriber.GenerateOPc(ki, op)
		if err != nil {
			return strings.ToLower(strings.TrimSpace(in.OP)), "ue.op"
		}
		return hex.EncodeToString(opc[:]), "ue.op"
	}
	return "", ""
}

func sameHexSecret(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func secretLabel(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if !hex32Pattern.MatchString(v) {
		return "provided value"
	}
	sum := sha256.Sum256([]byte(v))
	return "sha256:" + hex.EncodeToString(sum[:])[:10]
}

func normalizePLMN(v string) string {
	v = strings.TrimSpace(strings.Trim(v, `"'`))
	v = strings.ReplaceAll(v, "/", "")
	v = strings.ReplaceAll(v, "-", "")
	return v
}

func humanPLMN(v string) string {
	v = normalizePLMN(v)
	if len(v) < 5 {
		return v
	}
	return v[:3] + "/" + v[3:]
}

func normalizeSD(v string) string {
	v = strings.ToLower(strings.TrimSpace(strings.Trim(v, `"'`)))
	v = strings.TrimPrefix(v, "0x")
	if v == "" {
		return ""
	}
	for len(v) < 6 {
		v = "0" + v
	}
	return v
}

func normalizeScheme(v string) string {
	v = strings.ToLower(strings.TrimSpace(strings.Trim(v, `"'`)))
	switch v {
	case "", "0", "null", "null-scheme", "scheme0", "profile0":
		return v
	default:
		return v
	}
}

func coreSummary(core ReconcileCore) map[string]string {
	out := map[string]string{
		"plmn":                 humanPLMN(core.PLMN),
		"tac":                  fmt.Sprintf("%d", core.TAC),
		"s-nssai":              snssaiList(core.SNSSAIs),
		"serving_network_name": core.ServingNetworkName,
		"dnn":                  core.DefaultDNN,
	}
	if core.Subscriber != nil {
		out["subscriber_imsi"] = core.Subscriber.IMSI
		if core.Subscriber.APN != "" {
			out["dnn"] = core.Subscriber.APN
		}
	}
	return out
}

func firstValue(m map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case string:
			return strings.TrimSpace(x)
		case int:
			return strconv.Itoa(x)
		case int64:
			return strconv.FormatInt(x, 10)
		case uint64:
			return strconv.FormatUint(x, 10)
		case float64:
			return strconv.FormatUint(uint64(x), 10)
		}
	}
	return ""
}

func firstMap(m map[string]interface{}, keys ...string) map[string]interface{} {
	for _, key := range keys {
		if v, ok := m[key].(map[string]interface{}); ok {
			return v
		}
	}
	return nil
}

func uint16FromAny(v interface{}) (uint16, bool) {
	n, ok := uint64FromAny(v)
	if !ok || n > 0xffff {
		return 0, false
	}
	return uint16(n), true
}

func uint64FromAny(v interface{}) (uint64, bool) {
	switch x := v.(type) {
	case int:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	case int64:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	case uint64:
		return x, true
	case float64:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	case string:
		s := strings.TrimSpace(strings.Trim(x, `"'`))
		if s == "" {
			return 0, false
		}
		base := 10
		if strings.HasPrefix(strings.ToLower(s), "0x") {
			base = 16
			s = s[2:]
		}
		n, err := strconv.ParseUint(s, base, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
