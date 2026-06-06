package ngap

import (
	"encoding/binary"
	"fmt"
)

const maxNGAPBitRate uint64 = 4000000000000

// This file provides two forms of each complex IE:
//
//  writeX(enc, v) — writes the IE inline into an existing encoder (used when X
//                   is a direct SEQUENCE member; no extra alignment byte inserted).
//  EncodeX(v) []byte — creates a fresh encoder, calls writeX, returns the bytes
//                      (used when X is a standalone ProtocolIE value).
//
// In ALIGNED PER, direct SEQUENCE members are bit-packed consecutively with no
// intervening alignment. Wrapping with PutBytes / PutOctetString inserts a
// spurious alignment byte and breaks wire compatibility.

// --- GUAMI -------------------------------------------------------------------

func writeGUAMI(enc *PEREncoder, g GUAMI) {
	// GUAMI ::= SEQUENCE { pLMNIdentity, aMFRegionID, aMFSetID, aMFPointer,
	//                      iE-Extensions OPTIONAL, ... }
	enc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional (iE-Ext), absent
	enc.PutFixedOctetString(g.PLMN[:])
	enc.PutFixedOctetString([]byte{g.AMFRegionID})
	// AMFSetID(10 bits) + AMFPointer(6 bits) packed into 2 bytes
	enc.PutFixedOctetString([]byte{
		uint8(g.AMFSetID >> 2),
		uint8((g.AMFSetID&0x03)<<6) | (g.AMFPointer & 0x3F),
	})
}

func readGUAMI(dec *PERDecoder) (GUAMI, error) {
	var g GUAMI
	if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
		return g, fmt.Errorf("GUAMI seq: %w", err)
	}
	plmn, err := dec.GetFixedOctetString(3)
	if err != nil {
		return g, fmt.Errorf("GUAMI PLMN: %w", err)
	}
	copy(g.PLMN[:], plmn)
	region, err := dec.GetFixedOctetString(1)
	if err != nil {
		return g, fmt.Errorf("GUAMI AMFRegionID: %w", err)
	}
	g.AMFRegionID = region[0]
	sp, err := dec.GetFixedOctetString(2)
	if err != nil {
		return g, fmt.Errorf("GUAMI AMFSetID+Pointer: %w", err)
	}
	g.AMFSetID = uint16(sp[0])<<2 | uint16(sp[1])>>6
	g.AMFPointer = sp[1] & 0x3F
	return g, nil
}

// EncodeGUAMI encodes a GUAMI as a standalone ProtocolIE value.
func EncodeGUAMI(g GUAMI) []byte {
	enc := NewPEREncoder()
	writeGUAMI(enc, g)
	return enc.Bytes()
}

// DecodeGUAMI decodes a GUAMI from standalone ProtocolIE value bytes.
func DecodeGUAMI(data []byte) (GUAMI, error) {
	return readGUAMI(NewPERDecoder(data))
}

// --- TAI (5G, 3-byte TAC) ----------------------------------------------------

func writeTAI(enc *PEREncoder, tai TAI) {
	// TAI ::= SEQUENCE { pLMNIdentity PLMNIdentity, tAC TAC, iE-Extensions OPTIONAL, ... }
	// TAC ::= OCTET STRING SIZE(3)
	enc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional (iE-Ext), absent
	enc.PutFixedOctetString(tai.PLMN[:])
	enc.PutFixedOctetString(tai.TAC[:])
}

func readTAI(dec *PERDecoder) (TAI, error) {
	var tai TAI
	if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
		return tai, fmt.Errorf("TAI seq: %w", err)
	}
	plmn, err := dec.GetFixedOctetString(3)
	if err != nil {
		return tai, fmt.Errorf("TAI PLMN: %w", err)
	}
	copy(tai.PLMN[:], plmn)
	tac, err := dec.GetFixedOctetString(3)
	if err != nil {
		return tai, fmt.Errorf("TAI TAC: %w", err)
	}
	copy(tai.TAC[:], tac)
	return tai, nil
}

// EncodeTAI encodes a 5G TAI as a standalone ProtocolIE value.
func EncodeTAI(tai TAI) []byte {
	enc := NewPEREncoder()
	writeTAI(enc, tai)
	return enc.Bytes()
}

// DecodeTAI decodes a 5G TAI from standalone ProtocolIE value bytes.
func DecodeTAI(data []byte) (TAI, error) {
	return readTAI(NewPERDecoder(data))
}

// --- NR-CGI ------------------------------------------------------------------

func writeNRCGI(enc *PEREncoder, cgi NRCGI) {
	// NR-CGI ::= SEQUENCE { pLMNIdentity, nRCellIdentity BIT STRING(36), iE-Ext OPTIONAL, ... }
	enc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional (iE-Ext), absent
	enc.PutFixedOctetString(cgi.PLMN[:])
	// NRCellIdentity: 36-bit BIT STRING — 5 bytes (4 unused low bits)
	b := [5]byte{
		uint8(cgi.NRCellID >> 28),
		uint8(cgi.NRCellID >> 20),
		uint8(cgi.NRCellID >> 12),
		uint8(cgi.NRCellID >> 4),
		uint8(cgi.NRCellID<<4) & 0xF0,
	}
	enc.PutBitString(b[:], 36)
}

func readNRCGI(dec *PERDecoder) (NRCGI, error) {
	var cgi NRCGI
	if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
		return cgi, fmt.Errorf("NR-CGI seq: %w", err)
	}
	plmn, err := dec.GetFixedOctetString(3)
	if err != nil {
		return cgi, fmt.Errorf("NR-CGI PLMN: %w", err)
	}
	copy(cgi.PLMN[:], plmn)
	b, err := dec.GetFixedOctetString(5)
	if err != nil {
		return cgi, fmt.Errorf("NR-CGI cellID: %w", err)
	}
	cgi.NRCellID = uint64(b[0])<<28 | uint64(b[1])<<20 | uint64(b[2])<<12 | uint64(b[3])<<4 | uint64(b[4])>>4
	return cgi, nil
}

// EncodeNRCGI encodes an NR-CGI as a standalone ProtocolIE value.
func EncodeNRCGI(cgi NRCGI) []byte {
	enc := NewPEREncoder()
	writeNRCGI(enc, cgi)
	return enc.Bytes()
}

// DecodeNRCGI decodes an NR-CGI from standalone ProtocolIE value bytes.
func DecodeNRCGI(data []byte) (NRCGI, error) {
	return readNRCGI(NewPERDecoder(data))
}

// --- UserLocationInformation (NR variant) ------------------------------------

// EncodeUserLocationInformationNR encodes the UserLocationInformation IE value
// for a 5G NR cell, including the CHOICE wrapper.
//
//	UserLocationInformation ::= CHOICE {
//	  userLocationInformationEUTRA (0),
//	  userLocationInformationNR    (1),   ← we always encode this
//	  userLocationInformationN3IWF (2),
//	  ...
//	}
func EncodeUserLocationInformationNR(loc UserLocationInformationNR) []byte {
	enc := NewPEREncoder()
	_ = enc.PutChoiceIndex(1, 4, false) // NR = index 1

	// UserLocationInformationNR SEQUENCE: extensible, 2 optionals (timeStamp, iE-Ext), both absent
	enc.PutSequenceHeader(true, 0, 2)

	// nR-CGI inline (not length-prefixed)
	writeNRCGI(enc, loc.NRCGI)

	// tAI inline
	writeTAI(enc, loc.TAI)

	return enc.Bytes()
}

// DecodeUserLocationInformationNR decodes a UserLocationInformation IE value
// and returns the NR variant. Returns an error if the CHOICE is not NR.
func DecodeUserLocationInformationNR(data []byte) (UserLocationInformationNR, error) {
	dec := NewPERDecoder(data)
	var loc UserLocationInformationNR

	idx, err := dec.GetChoiceIndex(4, false)
	if err != nil {
		return loc, fmt.Errorf("ngap: UserLocationInfo index: %w", err)
	}
	if idx != 1 {
		return loc, fmt.Errorf("ngap: UserLocationInfo: expected NR (1), got %d", idx)
	}

	// UserLocationInformationNR SEQUENCE header
	if _, _, err := dec.GetSequenceHeader(true, 2); err != nil {
		return loc, fmt.Errorf("ngap: UserLocationInfoNR seq: %w", err)
	}

	// nR-CGI inline
	loc.NRCGI, err = readNRCGI(dec)
	if err != nil {
		return loc, fmt.Errorf("ngap: UserLocationInfo NR-CGI: %w", err)
	}

	// tAI inline
	loc.TAI, err = readTAI(dec)
	if err != nil {
		return loc, fmt.Errorf("ngap: UserLocationInfo TAI: %w", err)
	}

	return loc, nil
}

// --- GlobalGNB-ID ------------------------------------------------------------

// EncodeGlobalGNBID encodes a GlobalGNB-ID IE value.
//
// GlobalGNB-ID ::= SEQUENCE { pLMNIdentity, gNB-ID CHOICE { gNB-ID BIT STRING(22..32), ... } }
// gNB-ID is extensible CHOICE with 1 root alternative; wire = ext_bit(0) + constrained length + value.
func writeGlobalGNBID(enc *PEREncoder, g GlobalGNBID) error {
	bits := g.GNBIDBits
	if bits == 0 {
		bits = 32
	}
	if bits < 22 || bits > 32 {
		return fmt.Errorf("ngap: gNB-ID bit length %d out of range [22,32]", bits)
	}

	enc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional (iE-Ext), absent
	enc.PutFixedOctetString(g.PLMN[:])

	// GNB-ID CHOICE: ext bit (0), no index (1 root alt)
	enc.putBits(0, 1)

	// gNB-ID BIT STRING SIZE(22..32): constrained length (22..32, 4 bits) then value
	if err := enc.PutConstrainedInt(int64(bits), 22, 32); err != nil {
		return fmt.Errorf("ngap: gNB-ID length: %w", err)
	}
	byteLen := (int(bits) + 7) / 8
	enc.align()
	b := make([]byte, byteLen)
	switch byteLen {
	case 3:
		b[0] = uint8(g.GNBID >> 16)
		b[1] = uint8(g.GNBID >> 8)
		b[2] = uint8(g.GNBID)
	case 4:
		binary.BigEndian.PutUint32(b, g.GNBID)
	default:
		for i := byteLen - 1; i >= 0; i-- {
			b[i] = uint8(g.GNBID >> (uint(byteLen-1-i) * 8))
		}
	}
	if pad := uint(byteLen*8) - uint(bits); pad > 0 {
		b[byteLen-1] &^= (1 << pad) - 1
	}
	enc.PutBytes(b)
	return nil
}

func readGlobalGNBID(dec *PERDecoder) (GlobalGNBID, error) {
	var g GlobalGNBID

	if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
		return g, fmt.Errorf("ngap: GlobalGNB-ID seq: %w", err)
	}
	plmn, err := dec.GetFixedOctetString(3)
	if err != nil {
		return g, fmt.Errorf("ngap: GlobalGNB-ID PLMN: %w", err)
	}
	copy(g.PLMN[:], plmn)

	ext, err := dec.getBits(1)
	if err != nil {
		return g, fmt.Errorf("ngap: GNB-ID choice ext: %w", err)
	}
	if ext == 1 {
		return g, fmt.Errorf("ngap: GNB-ID extension not supported")
	}

	bits, err := dec.GetConstrainedInt(22, 32)
	if err != nil {
		return g, fmt.Errorf("ngap: gNB-ID length: %w", err)
	}
	g.GNBIDBits = uint8(bits)

	dec.align()
	byteLen := (int(bits) + 7) / 8
	b, err := dec.GetBytes(byteLen)
	if err != nil {
		return g, fmt.Errorf("ngap: gNB-ID value: %w", err)
	}
	for _, x := range b {
		g.GNBID = (g.GNBID << 8) | uint32(x)
	}
	// Shift right to remove padding bits
	if pad := uint(byteLen*8) - uint(bits); pad > 0 {
		g.GNBID >>= pad
	}
	return g, nil
}

// EncodeGlobalGNBID encodes a GlobalGNB-ID IE value.
func EncodeGlobalGNBID(g GlobalGNBID) ([]byte, error) {
	enc := NewPEREncoder()
	if err := writeGlobalGNBID(enc, g); err != nil {
		return nil, err
	}
	return enc.Bytes(), nil
}

// DecodeGlobalGNBID decodes a GlobalGNB-ID IE value.
func DecodeGlobalGNBID(data []byte) (GlobalGNBID, error) {
	return readGlobalGNBID(NewPERDecoder(data))
}

// --- ServedGUAMIList ---------------------------------------------------------

// EncodeServedGUAMIList encodes a ServedGUAMIList IE value.
// ServedGUAMIList ::= SEQUENCE(SIZE(1..256)) OF ServedGUAMIItem
func EncodeServedGUAMIList(items []ServedGUAMIItem) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("ngap: ServedGUAMIList must have at least one item")
	}
	enc := NewPEREncoder()
	if err := enc.PutConstrainedInt(int64(len(items)), 1, 256); err != nil {
		return nil, err
	}
	for _, item := range items {
		// ServedGUAMIItem SEQUENCE: extensible, 2 optionals (backupAMFName, iE-Ext)
		backupPresent := item.BackupAMFName != ""
		var optBits uint64
		if backupPresent {
			optBits = 2 // first optional field, MSB of 2-bit map
		}
		enc.PutSequenceHeader(true, optBits, 2)
		writeGUAMI(enc, item.GUAMI) // inline — no length prefix
		if backupPresent {
			if err := enc.PutPrintableString(item.BackupAMFName, 1, 150, true); err != nil {
				return nil, err
			}
		}
	}
	return enc.Bytes(), nil
}

// DecodeServedGUAMIList decodes a ServedGUAMIList IE value.
func DecodeServedGUAMIList(data []byte) ([]ServedGUAMIItem, error) {
	dec := NewPERDecoder(data)

	count, err := dec.GetConstrainedInt(1, 256)
	if err != nil {
		return nil, fmt.Errorf("ngap: ServedGUAMIList count: %w", err)
	}
	items := make([]ServedGUAMIItem, count)
	for i := range items {
		_, opts, err := dec.GetSequenceHeader(true, 2)
		if err != nil {
			return nil, fmt.Errorf("ngap: ServedGUAMIItem[%d] header: %w", i, err)
		}
		items[i].GUAMI, err = readGUAMI(dec)
		if err != nil {
			return nil, fmt.Errorf("ngap: ServedGUAMIItem[%d] GUAMI: %w", i, err)
		}
		if opts&2 != 0 {
			name, err := dec.GetPrintableString(1, 150, true)
			if err != nil {
				return nil, fmt.Errorf("ngap: ServedGUAMIItem[%d] backupAMFName: %w", i, err)
			}
			items[i].BackupAMFName = name
		}
	}
	return items, nil
}

// --- SupportedTAList ---------------------------------------------------------

// EncodeSupportedTAList encodes a SupportedTAList IE value.
func EncodeSupportedTAList(tas []SupportedTA) ([]byte, error) {
	if len(tas) == 0 {
		return nil, fmt.Errorf("ngap: SupportedTAList must have at least one item")
	}
	enc := NewPEREncoder()
	if err := enc.PutConstrainedInt(int64(len(tas)), 1, 256); err != nil {
		return nil, err
	}
	for _, ta := range tas {
		// SupportedTAItem SEQUENCE: extensible, 1 optional (iE-Ext), absent
		enc.PutSequenceHeader(true, 0, 1)
		enc.PutFixedOctetString(ta.TAC[:])
		// broadcastPLMNList: SEQUENCE(SIZE(1..12)) OF BroadcastPLMNItem
		if err := enc.PutConstrainedInt(int64(len(ta.BroadcastPLMN)), 1, 12); err != nil {
			return nil, err
		}
		for _, bp := range ta.BroadcastPLMN {
			enc.PutSequenceHeader(true, 0, 1)
			enc.PutFixedOctetString(bp.PLMN[:])
			// tAISliceSupportList: SEQUENCE(SIZE(1..1024)) OF SliceSupportItem
			if err := enc.PutConstrainedInt(int64(len(bp.SNSSAI)), 1, 1024); err != nil {
				return nil, err
			}
			for _, nssai := range bp.SNSSAI {
				sdPresent := len(nssai.SD) == 3
				// SliceSupportItem SEQUENCE: extensible, 1 optional (iE-Ext), absent
				enc.PutSequenceHeader(true, 0, 1)
				// S-NSSAI SEQUENCE: extensible, 2 optionals (sD, iE-Ext)
				var optBits uint64
				if sdPresent {
					optBits = 2
				}
				enc.PutSequenceHeader(true, optBits, 2)
				enc.PutFixedOctetString([]byte{nssai.SST})
				if sdPresent {
					enc.PutFixedOctetString(nssai.SD[:3])
				}
			}
		}
	}
	return enc.Bytes(), nil
}

// DecodeSupportedTAList decodes a SupportedTAList IE value.
func DecodeSupportedTAList(data []byte) ([]SupportedTA, error) {
	dec := NewPERDecoder(data)

	count, err := dec.GetConstrainedInt(1, 256)
	if err != nil {
		return nil, fmt.Errorf("ngap: SupportedTAList count: %w", err)
	}
	tas := make([]SupportedTA, count)
	for i := range tas {
		if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
			return nil, fmt.Errorf("ngap: SupportedTAItem[%d] header: %w", i, err)
		}
		tac, err := dec.GetFixedOctetString(3)
		if err != nil {
			return nil, fmt.Errorf("ngap: SupportedTAItem[%d] TAC: %w", i, err)
		}
		copy(tas[i].TAC[:], tac)

		bplmnCount, err := dec.GetConstrainedInt(1, 12)
		if err != nil {
			return nil, fmt.Errorf("ngap: SupportedTAItem[%d] PLMN count: %w", i, err)
		}
		tas[i].BroadcastPLMN = make([]BroadcastPLMNItem, bplmnCount)
		for j := range tas[i].BroadcastPLMN {
			if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
				return nil, fmt.Errorf("ngap: BroadcastPLMNItem[%d][%d] header: %w", i, j, err)
			}
			plmn, err := dec.GetFixedOctetString(3)
			if err != nil {
				return nil, fmt.Errorf("ngap: BroadcastPLMNItem[%d][%d] PLMN: %w", i, j, err)
			}
			copy(tas[i].BroadcastPLMN[j].PLMN[:], plmn)

			nssaiCount, err := dec.GetConstrainedInt(1, 1024)
			if err != nil {
				return nil, fmt.Errorf("ngap: SliceSupportList count: %w", err)
			}
			tas[i].BroadcastPLMN[j].SNSSAI = make([]SNSSAI, nssaiCount)
			for k := range tas[i].BroadcastPLMN[j].SNSSAI {
				if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
					return nil, fmt.Errorf("ngap: SliceSupportItem header: %w", err)
				}
				_, opts, err := dec.GetSequenceHeader(true, 2)
				if err != nil {
					return nil, fmt.Errorf("ngap: S-NSSAI header: %w", err)
				}
				sst, err := dec.GetFixedOctetString(1)
				if err != nil {
					return nil, fmt.Errorf("ngap: S-NSSAI SST: %w", err)
				}
				tas[i].BroadcastPLMN[j].SNSSAI[k].SST = sst[0]
				if opts&2 != 0 {
					sd, err := dec.GetFixedOctetString(3)
					if err != nil {
						return nil, fmt.Errorf("ngap: S-NSSAI SD: %w", err)
					}
					tas[i].BroadcastPLMN[j].SNSSAI[k].SD = sd
				}
			}
		}
	}
	return tas, nil
}

// --- PLMNSupportList ---------------------------------------------------------

// EncodePLMNSupportList encodes a PLMNSupportList IE value (NGSetup Response).
func EncodePLMNSupportList(items []PLMNSupportItem) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("ngap: PLMNSupportList must have at least one item")
	}
	enc := NewPEREncoder()
	if err := enc.PutConstrainedInt(int64(len(items)), 1, 12); err != nil {
		return nil, err
	}
	for _, item := range items {
		enc.PutSequenceHeader(true, 0, 1)
		enc.PutFixedOctetString(item.PLMN[:])
		if err := enc.PutConstrainedInt(int64(len(item.SNSSAIs)), 1, 1024); err != nil {
			return nil, err
		}
		for _, nssai := range item.SNSSAIs {
			sdPresent := len(nssai.SD) == 3
			enc.PutSequenceHeader(true, 0, 1) // SliceSupportItem
			var optBits uint64
			if sdPresent {
				optBits = 2 // first optional field, MSB of 2-bit map
			}
			enc.PutSequenceHeader(true, optBits, 2) // S-NSSAI
			enc.PutFixedOctetString([]byte{nssai.SST})
			if sdPresent {
				enc.PutFixedOctetString(nssai.SD[:3])
			}
		}
	}
	return enc.Bytes(), nil
}

// --- AMF/RAN UE NGAP IDs ----------------------------------------------------

// EncodeAMFUENGAPID encodes an AMF-UE-NGAP-ID IE value (INTEGER 0..1099511627775).
func EncodeAMFUENGAPID(id uint64) ([]byte, error) {
	return encodeUENGAPID(id, 1099511627775)
}

// DecodeAMFUENGAPID decodes an AMF-UE-NGAP-ID IE value.
func DecodeAMFUENGAPID(data []byte) (uint64, error) {
	return decodeUENGAPID(data, 1099511627775)
}

// EncodeRANUENGAPID encodes a RAN-UE-NGAP-ID IE value (INTEGER 0..4294967295).
func EncodeRANUENGAPID(id uint64) ([]byte, error) {
	return encodeUENGAPID(id, 4294967295)
}

// DecodeRANUENGAPID decodes a RAN-UE-NGAP-ID IE value.
func DecodeRANUENGAPID(data []byte) (uint64, error) {
	return decodeUENGAPID(data, 4294967295)
}

func encodeUENGAPID(id, max uint64) ([]byte, error) {
	if id > max {
		return nil, fmt.Errorf("value %d out of range [0, %d]", id, max)
	}
	width := 2
	for tmp := id; tmp > 0xffff; tmp >>= 8 {
		width++
	}
	maxWidth := 4
	if max > 0xffffffff {
		maxWidth = 5
	}
	if width > maxWidth {
		width = maxWidth
	}
	out := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		out[i] = uint8(id)
		id >>= 8
	}
	return out, nil
}

func decodeUENGAPID(data []byte, max uint64) (uint64, error) {
	maxWidth := 4
	if max > 0xffffffff {
		maxWidth = 5
	}
	if len(data) == 0 || len(data) > maxWidth {
		return 0, fmt.Errorf("need 1..%d bytes, have %d", maxWidth, len(data))
	}
	var id uint64
	for _, b := range data {
		id = (id << 8) | uint64(b)
	}
	if id > max {
		return 0, fmt.Errorf("value %d out of range [0, %d]", id, max)
	}
	return id, nil
}

// --- Cause -------------------------------------------------------------------

// EncodeCause encodes a Cause IE value.
// Cause ::= CHOICE { radioNetwork(0), transport(1), nas(2), protocol(3), misc(4), ... }
func EncodeCause(group CauseGroup, value uint8) []byte {
	enc := NewPEREncoder()
	_ = enc.PutChoiceIndex(int(group), 5, true)
	enc.align()
	enc.PutBytes([]byte{value})
	return enc.Bytes()
}

// DecodeCause decodes a Cause IE value.
func DecodeCause(data []byte) (CauseGroup, uint8, error) {
	dec := NewPERDecoder(data)
	idx, err := dec.GetChoiceIndex(5, true)
	if err != nil {
		return 0, 0, err
	}
	dec.align()
	b, err := dec.GetBytes(1)
	if err != nil {
		return 0, 0, err
	}
	return CauseGroup(idx), b[0], nil
}

// --- UE Aggregate Maximum Bit Rate -------------------------------------------

// EncodeUEAggregateMaximumBitRate encodes UEAggregateMaximumBitRate.
//
//	UEAggregateMaximumBitRate ::= SEQUENCE {
//	  uEAggregateMaximumBitRateDL BitRate,
//	  uEAggregateMaximumBitRateUL BitRate,
//	  iE-Extensions OPTIONAL,
//	  ...
//	}
//
// BitRate ::= INTEGER (0..4000000000000,...). asn1c's aligned PER path emits
// a one-bit extension marker, a 3-bit byte-count prefix for the 42-bit range,
// then the byte-aligned minimum-width integer.
func EncodeUEAggregateMaximumBitRate(dl, ul uint64) ([]byte, error) {
	enc := NewPEREncoder()
	enc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional (iE-Ext), absent
	if err := putBitRate(enc, dl); err != nil {
		return nil, fmt.Errorf("DL: %w", err)
	}
	if err := putBitRate(enc, ul); err != nil {
		return nil, fmt.Errorf("UL: %w", err)
	}
	return enc.Bytes(), nil
}

func DecodeUEAggregateMaximumBitRate(data []byte) (dl, ul uint64, err error) {
	dec := NewPERDecoder(data)
	if _, _, err = dec.GetSequenceHeader(true, 1); err != nil {
		return 0, 0, err
	}
	if dl, err = getBitRate(dec); err != nil {
		return 0, 0, fmt.Errorf("DL: %w", err)
	}
	if ul, err = getBitRate(dec); err != nil {
		return 0, 0, fmt.Errorf("UL: %w", err)
	}
	return dl, ul, nil
}

func putBitRate(enc *PEREncoder, v uint64) error {
	if v > maxNGAPBitRate {
		return fmt.Errorf("bitrate %d out of range [0,%d]", v, maxNGAPBitRate)
	}
	enc.PutBool(false) // BitRate extension marker: root value.
	width := 1
	for tmp := v; tmp > 0xff; tmp >>= 8 {
		width++
	}
	enc.putBits(uint8(width-1), 3) // max root width is 6 bytes for 42 bits.
	enc.align()
	b := make([]byte, width)
	for i := len(b) - 1; i >= 0; i-- {
		b[i] = byte(v)
		v >>= 8
	}
	enc.PutBytes(b)
	return nil
}

func getBitRate(dec *PERDecoder) (uint64, error) {
	extended, err := dec.GetBool()
	if err != nil {
		return 0, err
	}
	if extended {
		return 0, fmt.Errorf("extended BitRate values are not supported")
	}
	widthMinusOne, err := dec.GetBits(3)
	if err != nil {
		return 0, err
	}
	width := int(widthMinusOne) + 1
	if width > 6 {
		return 0, fmt.Errorf("BitRate width %d exceeds 42-bit root range", width)
	}
	dec.align()
	b, err := dec.GetBytes(width)
	if err != nil {
		return 0, err
	}
	var v uint64
	for _, x := range b {
		v = (v << 8) | uint64(x)
	}
	if v > maxNGAPBitRate {
		return 0, fmt.Errorf("bitrate %d out of range [0,%d]", v, maxNGAPBitRate)
	}
	return v, nil
}

// --- UESecurityCapabilities --------------------------------------------------

// EncodeUESecurityCapabilities encodes a UESecurityCapabilities IE value.
// The four 16-bit BIT STRINGs carry NR and E-UTRA algorithm bitmaps.
func EncodeUESecurityCapabilities(nrEnc, nrInt, eutraEnc, eutraInt [2]byte) []byte {
	enc := NewPEREncoder()
	enc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional (iE-Ext), absent
	enc.PutFixedOctetString(nrEnc[:])
	enc.PutFixedOctetString(nrInt[:])
	enc.PutFixedOctetString(eutraEnc[:])
	enc.PutFixedOctetString(eutraInt[:])
	return enc.Bytes()
}

// DecodeUESecurityCapabilities decodes a UESecurityCapabilities IE value.
func DecodeUESecurityCapabilities(data []byte) (nrEnc, nrInt, eutraEnc, eutraInt [2]byte, err error) {
	dec := NewPERDecoder(data)
	if _, _, err = dec.GetSequenceHeader(true, 1); err != nil {
		return
	}
	for _, dst := range []*[2]byte{&nrEnc, &nrInt, &eutraEnc, &eutraInt} {
		var b []byte
		b, err = dec.GetFixedOctetString(2)
		if err != nil {
			return
		}
		copy(dst[:], b)
	}
	return
}
