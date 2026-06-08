package amf

import (
	"sync"

	"github.com/qcore-project/qcore/pkg/ngap"
)

// UEState models the UE's registration state machine.
type UEState int

const (
	StateIdle           UEState = iota
	StateAuthPending            // sent Auth Request, waiting for Auth Response
	StateSecModePending         // AUSF confirmed, sent SMC, waiting for SMC Complete
	StateRegistered             // sent Registration Accept, received Registration Complete
)

// UEContext holds all per-UE state across the NGAP + NAS flows.
type UEContext struct {
	mu sync.Mutex

	State UEState

	// NGAP identifiers
	AMFUENGAPID uint64
	RANUENGAPID uint64

	// Back-reference to the gNB session carrying this UE
	gNB *gNBSession

	// JourneyID is minted at first contact and propagated everywhere
	JourneyID string

	// 5G identifiers
	SUPI string // filled after auth confirmation
	SUCI []byte // raw mobile identity bytes from Registration Request
	GUTI *ngap.GUAMI

	// Authentication
	AuthCtxURL string // AUSF 5g-aka-confirmation URL
	RAND       [16]byte
	AUTN       [16]byte

	// Keys (filled after AUSF confirmation)
	KAMF    []byte // 256-bit
	KNASint []byte // 128-bit
	KNASenc []byte // 128-bit
	KgNB    []byte // 256-bit

	// NAS security
	IntAlgID uint8 // 2 = NIA2
	EncAlgID uint8 // 0 = NEA0
	DLCount  uint32
	ULCount  uint32

	// Capabilities
	UESecCaps []byte // raw UE security capability IE value
	NSSAI     []ngap.SNSSAI

	// Location (last known)
	UserLocation ngap.UserLocationInformationNR

	// PDU session state learned during T10 interop.
	PDUSessionID uint8
	SMContextRef string
	SMFURL       string
}

func newUEContext(amfID, ranID uint64, gNB *gNBSession) *UEContext {
	return &UEContext{
		AMFUENGAPID: amfID,
		RANUENGAPID: ranID,
		gNB:         gNB,
		State:       StateIdle,
		IntAlgID:    2, // NIA2
		EncAlgID:    0, // NEA0 (null)
	}
}
