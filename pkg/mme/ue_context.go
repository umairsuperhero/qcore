package mme

import (
	"sync"
)

// EMMState represents EMM (EPS Mobility Management) state per TS 24.301.
type EMMState int

const (
	EMMDeregistered         EMMState = iota
	EMMRegisterInitiated             // Attach in progress
	EMMRegistered                    // Attached
	EMMDeregisterInitiated           // Detach in progress
)

func (s EMMState) String() string {
	switch s {
	case EMMDeregistered:
		return "Deregistered"
	case EMMRegisterInitiated:
		return "RegisterInitiated"
	case EMMRegistered:
		return "Registered"
	case EMMDeregisterInitiated:
		return "DeregisterInitiated"
	default:
		return "Unknown"
	}
}

// ECMState represents ECM (EPS Connection Management) state.
type ECMState int

const (
	ECMIdle      ECMState = iota
	ECMConnected
)

func (s ECMState) String() string {
	switch s {
	case ECMIdle:
		return "Idle"
	case ECMConnected:
		return "Connected"
	default:
		return "Unknown"
	}
}

// SecurityContext holds the derived keys for a UE session.
type SecurityContext struct {
	KASME    []byte // 32 bytes
	KNASenc  []byte // 16 bytes — NAS encryption key
	KNASint  []byte // 16 bytes — NAS integrity key
	KeNB     []byte // 32 bytes — eNodeB key
	NASAlg   NASAlgorithm
	ULCount  uint32 // Uplink NAS COUNT
	DLCount  uint32 // Downlink NAS COUNT
}

// NASAlgorithm identifies the selected NAS security algorithms.
type NASAlgorithm struct {
	Ciphering uint8 // 0=EEA0 (null), 1=EEA1 (SNOW), 2=EEA2 (AES)
	Integrity uint8 // 0=EIA0 (null), 1=EIA1 (SNOW), 2=EIA2 (AES-CMAC)
}

// UEContext tracks the state of a single UE within the MME.
type UEContext struct {
	mu sync.RWMutex

	// Identity
	IMSI    string
	GUTI    string // allocated by MME (e.g., "001-01-1-01-00000001")
	TMSI    uint32 // M-TMSI component of GUTI (non-zero once allocated)
	MSISDN  string

	// S1AP IDs
	MMEUES1APID uint32
	ENBUES1APID uint32

	// State
	EMMState EMMState
	ECMState ECMState

	// Security
	SecurityCtx *SecurityContext
	KASME       []byte // raw KASME bytes before keys are derived

	// Auth
	RAND []byte
	XRES []byte
	AUTN []byte

	// NAS layer
	NASStreamID        uint16 // SCTP stream for NAS transport
	NASdlCount         uint32 // NAS downlink count (for MAC computation)
	UENetworkCapability []byte // replayed in Security Mode Command

	// Network
	ENB     *EnbContext
	TAI     TAI
	ECGI    ECGI
	PDNAddr string // Allocated IPv4 address (dotted notation)
}

// TAI is a Tracking Area Identity.
type TAI struct {
	PLMN [3]byte
	TAC  uint16
}

// ECGI is an E-UTRAN Cell Global Identifier.
type ECGI struct {
	PLMN   [3]byte
	CellID uint32 // 28-bit
}

// SetEMMState transitions the UE's EMM state.
func (ue *UEContext) SetEMMState(state EMMState) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	ue.EMMState = state
}

// SetECMState transitions the UE's ECM state.
func (ue *UEContext) SetECMState(state ECMState) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	ue.ECMState = state
}
