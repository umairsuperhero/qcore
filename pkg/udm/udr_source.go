package udm

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/qcore-project/qcore/pkg/sbi/common"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/qcore-project/qcore/pkg/udr"
)

// NewUDRSource adapts a pkg/udr.Client to AmDataSource — the network-mode
// counterpart of NewStoreSource. Use this when UDM should not read the
// subscriber database directly but instead fetch over Nudr_DataRepository.
//
// defaultPlmnID is stamped into every outbound UDR URL (which requires a
// servingPlmnId path segment). QCore's UDR ignores it today, but a real
// deployment routes on it. "00101" is a safe placeholder for single-PLMN
// dev setups.
func NewUDRSource(c *udr.Client, defaultPlmnID string) AmDataSource {
	return &udrSource{c: c, plmnID: defaultPlmnID}
}

type udrSource struct {
	c      *udr.Client
	plmnID string
}

func (u *udrSource) GetAmData(ctx context.Context, supi string) (*common.AccessAndMobilitySubscriptionData, error) {
	// UDR's ueId form matches SUPI's imsi-<digits> form 1:1 — pass through.
	data, err := u.c.GetAmData(ctx, supi, u.plmnID)
	if err != nil {
		// Translate udr sentinels to udm sentinels so the HTTP handler
		// produces the same RFC 7807 Problem regardless of backend.
		switch {
		case errors.Is(err, udr.ErrNotFound):
			return nil, ErrNotFound
		case errors.Is(err, udr.ErrBadUeID):
			return nil, ErrBadSupi
		}
		return nil, err
	}
	return data, nil
}

// NewUDRAuthSource adapts a pkg/udr.Client to AuthSource — the
// network-mode counterpart of NewStoreAuthSource. The flow on each
// GenerateAv call is:
//
//  1. GET /authentication-data/authentication-subscription from UDR.
//  2. Run Milenage + Annex A derivations locally from the returned
//     K/OPc/AMF/SQN.
//  3. Increment SQN and PATCH it back to UDR so the next call won't
//     replay the same AUTN.
//
// Step 3 is a new round-trip per vector — fine for v0.5 scale. A future
// cut can batch-reserve a range of SQNs per PATCH to cut the RTT cost.
func NewUDRAuthSource(c *udr.Client) AuthSource {
	return &udrAuthSource{c: c}
}

type udrAuthSource struct {
	c *udr.Client
}

func (u *udrAuthSource) GenerateAv(ctx context.Context, supi, snName string) (*Av5gHeAka, error) {
	// SUPI's imsi-<15-digits> form doubles as UDR's ueId for the IMSI case.
	authSub, err := u.c.GetAuthenticationSubscription(ctx, supi)
	if err != nil {
		switch {
		case errors.Is(err, udr.ErrNotFound):
			return nil, ErrNotFound
		case errors.Is(err, udr.ErrBadUeID):
			return nil, ErrBadSupi
		}
		return nil, fmt.Errorf("udr: fetch auth-subscription: %w", err)
	}
	if authSub.SequenceNumber == nil {
		return nil, fmt.Errorf("udr returned auth-subscription without sequenceNumber")
	}

	k, err := decode16Hex(authSub.EncPermanentKey, "encPermanentKey")
	if err != nil {
		return nil, err
	}
	opc, err := decode16Hex(authSub.EncOpcKey, "encOpcKey")
	if err != nil {
		return nil, err
	}
	sqn, err := decode6Hex(authSub.SequenceNumber.Sqn, "sqn")
	if err != nil {
		return nil, err
	}
	amf, err := decode2Hex(authSub.AuthenticationManagementField, "authenticationManagementField")
	if err != nil {
		return nil, err
	}

	av, err := subscriber.Generate5GAuthVector(k, opc, sqn, amf, snName)
	if err != nil {
		return nil, fmt.Errorf("milenage: %w", err)
	}

	// Advance SQN and persist, so the next vector for this SUPI carries
	// a fresh AUTN. If the PATCH fails the caller will see a 5xx and no
	// vector — better than handing out a vector the UE will reject later.
	nextSQN, err := subscriber.IncrementSQNHex(authSub.SequenceNumber.Sqn)
	if err != nil {
		return nil, fmt.Errorf("incrementing sqn: %w", err)
	}
	if err := u.c.UpdateAuthSubscriptionSQN(ctx, supi, nextSQN); err != nil {
		return nil, fmt.Errorf("udr: persist advanced sqn: %w", err)
	}

	return &Av5gHeAka{
		AvType:   "5G_HE_AKA",
		RAND:     av.RAND,
		XResStar: av.XRESStar,
		AUTN:     av.AUTN,
		KAUSF:    av.KAUSF,
	}, nil
}

func decode16Hex(s, field string) ([16]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 16 {
		return [16]byte{}, fmt.Errorf("%s must be 32 hex chars, got %q", field, s)
	}
	var out [16]byte
	copy(out[:], b)
	return out, nil
}

func decode6Hex(s, field string) ([6]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 6 {
		return [6]byte{}, fmt.Errorf("%s must be 12 hex chars, got %q", field, s)
	}
	var out [6]byte
	copy(out[:], b)
	return out, nil
}

func decode2Hex(s, field string) ([2]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 2 {
		return [2]byte{}, fmt.Errorf("%s must be 4 hex chars, got %q", field, s)
	}
	var out [2]byte
	copy(out[:], b)
	return out, nil
}
