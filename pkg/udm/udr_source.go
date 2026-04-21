package udm

import (
	"context"
	"errors"

	"github.com/qcore-project/qcore/pkg/sbi/common"
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
