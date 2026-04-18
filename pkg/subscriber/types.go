package subscriber

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"gorm.io/gorm"
)

var (
	imsiPattern = regexp.MustCompile(`^\d{15}$`)
	hexPattern  = regexp.MustCompile(`^[0-9a-fA-F]+$`)
)

type Subscriber struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	IMSI      string         `gorm:"uniqueIndex;size:15;not null" json:"imsi"`
	MSISDN    string         `gorm:"index;size:15" json:"msisdn,omitempty"`
	Ki        string         `gorm:"size:32;not null" json:"ki"`
	OPc       string         `gorm:"size:32;not null" json:"opc"`
	AMF       string         `gorm:"size:4;not null;default:'8000'" json:"amf"`
	SQN       string         `gorm:"size:12;not null;default:'000000000000'" json:"sqn"`
	PDNType   int            `gorm:"default:0" json:"pdn_type"`
	APN       string         `gorm:"size:100;default:'internet'" json:"apn"`
	Status    int            `gorm:"default:0" json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (s *Subscriber) Validate() error {
	if !imsiPattern.MatchString(s.IMSI) {
		return fmt.Errorf("IMSI must be exactly 15 digits, got %q (example: 001010000000001)", s.IMSI)
	}
	if len(s.Ki) != 32 || !hexPattern.MatchString(s.Ki) {
		return fmt.Errorf("Ki must be 32 hex characters (128-bit key), got %q (example: 465b5ce8b199b49faa5f0a2ee238a6bc)", s.Ki)
	}
	if len(s.OPc) != 32 || !hexPattern.MatchString(s.OPc) {
		return fmt.Errorf("OPc must be 32 hex characters (128-bit operator variant), got %q (derive with the OP and Ki, or use your SIM vendor's value)", s.OPc)
	}
	if len(s.AMF) != 4 || !hexPattern.MatchString(s.AMF) {
		return fmt.Errorf("AMF must be 4 hex characters, got %q (default: 8000)", s.AMF)
	}
	if len(s.SQN) != 12 || !hexPattern.MatchString(s.SQN) {
		return fmt.Errorf("SQN must be 12 hex characters, got %q (start with 000000000000 for a new SIM)", s.SQN)
	}
	return nil
}

func (s *Subscriber) KiBytes() ([16]byte, error) {
	return decodeHex16(s.Ki)
}

func (s *Subscriber) OPcBytes() ([16]byte, error) {
	return decodeHex16(s.OPc)
}

func (s *Subscriber) AMFBytes() ([2]byte, error) {
	b, err := hex.DecodeString(s.AMF)
	if err != nil {
		return [2]byte{}, fmt.Errorf("decoding AMF: %w", err)
	}
	if len(b) != 2 {
		return [2]byte{}, fmt.Errorf("AMF must decode to 2 bytes, got %d", len(b))
	}
	var result [2]byte
	copy(result[:], b)
	return result, nil
}

func (s *Subscriber) SQNBytes() ([6]byte, error) {
	b, err := hex.DecodeString(s.SQN)
	if err != nil {
		return [6]byte{}, fmt.Errorf("decoding SQN: %w", err)
	}
	if len(b) != 6 {
		return [6]byte{}, fmt.Errorf("SQN must decode to 6 bytes, got %d", len(b))
	}
	var result [6]byte
	copy(result[:], b)
	return result, nil
}

func (s *Subscriber) IncrementSQN() error {
	val, err := strconv.ParseUint(s.SQN, 16, 48)
	if err != nil {
		return fmt.Errorf("parsing SQN: %w", err)
	}
	val++
	if val > 0xffffffffffff {
		val = 0
	}
	s.SQN = fmt.Sprintf("%012x", val)
	return nil
}

func decodeHex16(s string) ([16]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return [16]byte{}, fmt.Errorf("decoding hex: %w", err)
	}
	if len(b) != 16 {
		return [16]byte{}, fmt.Errorf("expected 16 bytes, got %d", len(b))
	}
	var result [16]byte
	copy(result[:], b)
	return result, nil
}
