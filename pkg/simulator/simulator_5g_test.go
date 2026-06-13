package simulator

import (
	"testing"

	"github.com/qcore-project/qcore/pkg/ngap"
	"github.com/stretchr/testify/require"
)

func TestBuildNGSetupRequestIncludesSupportedSlice(t *testing.T) {
	req := buildNGSetupRequest(Options{
		Mode: "5g",
		PLMN: [3]byte{0x00, 0xF1, 0x10},
		TAC:  1,
	})

	require.Len(t, req.SupportedTAList, 1)
	require.Len(t, req.SupportedTAList[0].BroadcastPLMN, 1)
	require.Len(t, req.SupportedTAList[0].BroadcastPLMN[0].SNSSAI, 1)

	raw, err := ngap.EncodeNGSetupRequest(req)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}
