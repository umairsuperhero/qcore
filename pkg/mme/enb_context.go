package mme

import (
	"fmt"
	"sync"

	"github.com/qcore-project/qcore/pkg/sctp"
)

// EnbContext tracks a connected eNodeB.
type EnbContext struct {
	mu sync.RWMutex

	// Identity
	GlobalENBID  GlobalENBID
	ENBName      string
	SupportedTAs []SupportedTA

	// Transport
	Assoc sctp.Association

	// State
	PagingDRX PagingDRX
}

// GlobalENBID uniquely identifies an eNodeB within a PLMN.
type GlobalENBID struct {
	PLMN  [3]byte
	ENBID uint32 // 20-bit macro eNB ID or 28-bit home eNB ID
	Type  ENBIDType
}

type ENBIDType int

const (
	MacroENB ENBIDType = iota
	HomeENB
)

// SupportedTA represents a Tracking Area supported by an eNodeB.
type SupportedTA struct {
	TAC       uint16
	PLMNs     [][3]byte
}

// PagingDRX is the default paging DRX cycle length.
type PagingDRX int

const (
	PagingDRX32  PagingDRX = 0
	PagingDRX64  PagingDRX = 1
	PagingDRX128 PagingDRX = 2
	PagingDRX256 PagingDRX = 3
)

func (e *EnbContext) String() string {
	return fmt.Sprintf("eNB[%s id=0x%x]", e.ENBName, e.GlobalENBID.ENBID)
}
