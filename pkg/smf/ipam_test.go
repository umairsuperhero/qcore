package smf

import (
	"testing"
)

func TestIPAM(t *testing.T) {
	ipam, err := NewIPAM("10.0.0.0/29")
	if err != nil {
		t.Fatalf("Failed to init IPAM: %v", err)
	}

	// /29 has 8 IPs: .0 to .7
	// We skip .7 if it's broadast in our naive logic, so let's just allocate a few
	ip1, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if ip1.String() != "10.0.0.1" {
		t.Errorf("Expected 10.0.0.1, got %s", ip1.String())
	}

	ip2, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if ip2.String() != "10.0.0.2" {
		t.Errorf("Expected 10.0.0.2, got %s", ip2.String())
	}

	ipam.Release(ip1)

	// Allocate a bunch to exhaust
	for i := 0; i < 5; i++ {
		_, _ = ipam.Allocate()
	}

	// Should wrap around and give us the released one
	ip3, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate failed after wrap: %v", err)
	}
	if ip3.String() != "10.0.0.1" {
		t.Errorf("Expected 10.0.0.1 after release, got %s", ip3.String())
	}
}
