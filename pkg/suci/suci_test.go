package suci

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestDeconceal_AnnexC4Vectors(t *testing.T) {
	tests := []struct {
		name         string
		scheme       uint8
		privateKey   string
		schemeOutput string
		wantMSINBCD  string
		wantMSIN     string
	}{
		{
			name:       "profile A",
			scheme:     SchemeProfileA,
			privateKey: "c53c22208b61860b06c62e5406a7b330c2b577aa5558981510d128247d38bd1d",
			schemeOutput: "" +
				"b2e92f836055a255837debf850b528997ce0201cb82adfe4be1f587d07d8457d" +
				"cb02352410" +
				"cddd9e730ef3fa87",
			wantMSINBCD: "00012080f6",
			wantMSIN:    "001002086",
		},
		{
			name:       "profile B",
			scheme:     SchemeProfileB,
			privateKey: "F1AB1074477EBCC7F554EA1C5FC368B1616730155E0041AC447D6301975FECDA",
			schemeOutput: "" +
				"039AAB8376597021E855679A9778EA0B67396E68C66DF32C0F41E9ACCA2DA9B9D1" +
				"46A33FC271" +
				"6AC7DAE96AA30A4D",
			wantMSINBCD: "00012080f6",
			wantMSIN:    "001002086",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			priv, err := ParsePrivateKeyHex(tt.scheme, tt.privateKey)
			if err != nil {
				t.Fatalf("ParsePrivateKeyHex: %v", err)
			}
			out, err := Deconceal(tt.scheme, priv, mustHex(t, tt.schemeOutput))
			if err != nil {
				t.Fatalf("Deconceal: %v", err)
			}
			if got := hex.EncodeToString(out); got != strings.ToLower(tt.wantMSINBCD) {
				t.Fatalf("MSIN BCD: got %s, want %s", got, tt.wantMSINBCD)
			}
			msin, err := DecodeMSINBCD(out)
			if err != nil {
				t.Fatalf("DecodeMSINBCD: %v", err)
			}
			if msin != tt.wantMSIN {
				t.Fatalf("MSIN: got %s, want %s", msin, tt.wantMSIN)
			}
		})
	}
}

func TestDeconceal_RejectsBadMAC(t *testing.T) {
	priv, err := ParsePrivateKeyHex(SchemeProfileA, "c53c22208b61860b06c62e5406a7b330c2b577aa5558981510d128247d38bd1d")
	if err != nil {
		t.Fatalf("ParsePrivateKeyHex: %v", err)
	}
	schemeOutput := mustHex(t, ""+
		"b2e92f836055a255837debf850b528997ce0201cb82adfe4be1f587d07d8457d"+
		"cb02352410"+
		"cddd9e730ef3fa87")
	schemeOutput[len(schemeOutput)-1] ^= 0x01

	_, err = Deconceal(SchemeProfileA, priv, schemeOutput)
	if !errors.Is(err, ErrMACVerification) {
		t.Fatalf("Deconceal error: got %v, want ErrMACVerification", err)
	}
}

func TestResolver_ResolvesNullAndProfileASUCI(t *testing.T) {
	priv, err := ParsePrivateKeyHex(SchemeProfileA, "c53c22208b61860b06c62e5406a7b330c2b577aa5558981510d128247d38bd1d")
	if err != nil {
		t.Fatalf("ParsePrivateKeyHex: %v", err)
	}
	resolver, err := NewResolver([]HomeNetworkPrivateKey{{ID: 1, Scheme: SchemeProfileA, PrivateKey: priv}})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	const profileASUCI = "" +
		"f1" + "722410" + "0000" + "01" + "01" +
		"b2e92f836055a255837debf850b528997ce0201cb82adfe4be1f587d07d8457d" +
		"cb02352410" +
		"cddd9e730ef3fa87"
	got, err := resolver.ResolveSUCIHex("suci-" + profileASUCI)
	if err != nil {
		t.Fatalf("ResolveSUCIHex profile A: %v", err)
	}
	if got != "imsi-274012001002086" {
		t.Fatalf("profile A SUPI: got %s, want imsi-274012001002086", got)
	}

	got, err = resolver.ResolveSUCIHex("suci-f17224100000000000012080f6")
	if err != nil {
		t.Fatalf("ResolveSUCIHex null scheme: %v", err)
	}
	if got != "imsi-274012001002086" {
		t.Fatalf("null scheme SUPI: got %s, want imsi-274012001002086", got)
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode %q: %v", s, err)
	}
	return b
}
