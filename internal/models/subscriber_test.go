package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriber_Validate(t *testing.T) {
	tests := []struct {
		name    string
		sub     Subscriber
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid subscriber",
			sub: Subscriber{
				IMSI: "001010000000001",
				Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
				OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
				AMF:  "8000",
				SQN:  "000000000000",
			},
			wantErr: false,
		},
		{
			name: "IMSI too short",
			sub: Subscriber{
				IMSI: "00101",
				Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
				OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
				AMF:  "8000",
				SQN:  "000000000000",
			},
			wantErr: true,
			errMsg:  "IMSI must be exactly 15 digits",
		},
		{
			name: "IMSI with letters",
			sub: Subscriber{
				IMSI: "00101000000abc",
				Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
				OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
				AMF:  "8000",
				SQN:  "000000000000",
			},
			wantErr: true,
			errMsg:  "IMSI must be exactly 15 digits",
		},
		{
			name: "Ki too short",
			sub: Subscriber{
				IMSI: "001010000000001",
				Ki:   "465b5ce8",
				OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
				AMF:  "8000",
				SQN:  "000000000000",
			},
			wantErr: true,
			errMsg:  "Ki must be 32 hex characters",
		},
		{
			name: "Ki invalid hex",
			sub: Subscriber{
				IMSI: "001010000000001",
				Ki:   "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
				OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
				AMF:  "8000",
				SQN:  "000000000000",
			},
			wantErr: true,
			errMsg:  "Ki must be 32 hex characters",
		},
		{
			name: "OPc wrong length",
			sub: Subscriber{
				IMSI: "001010000000001",
				Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
				OPc:  "cd63cb71",
				AMF:  "8000",
				SQN:  "000000000000",
			},
			wantErr: true,
			errMsg:  "OPc must be 32 hex characters",
		},
		{
			name: "AMF wrong length",
			sub: Subscriber{
				IMSI: "001010000000001",
				Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
				OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
				AMF:  "80",
				SQN:  "000000000000",
			},
			wantErr: true,
			errMsg:  "AMF must be 4 hex characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sub.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSubscriber_KiBytes(t *testing.T) {
	sub := Subscriber{Ki: "465b5ce8b199b49faa5f0a2ee238a6bc"}
	ki, err := sub.KiBytes()
	require.NoError(t, err)
	assert.Equal(t, byte(0x46), ki[0])
	assert.Equal(t, byte(0xbc), ki[15])
}

func TestSubscriber_IncrementSQN(t *testing.T) {
	tests := []struct {
		name    string
		sqn     string
		wantSQN string
	}{
		{"zero", "000000000000", "000000000001"},
		{"normal", "000000000042", "000000000043"},
		{"overflow", "ffffffffffff", "000000000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := Subscriber{SQN: tt.sqn}
			err := sub.IncrementSQN()
			require.NoError(t, err)
			assert.Equal(t, tt.wantSQN, sub.SQN)
		})
	}
}
