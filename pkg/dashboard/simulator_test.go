package dashboard

import (
	"testing"

	"github.com/qcore-project/qcore/pkg/simulator"
)

func TestSimulatorControllerTemplateForMode(t *testing.T) {
	c := &SimulatorController{
		template4G: simulator.Options{
			Mode:          "4g",
			MMEAddr:       "mme:36412",
			TransportMode: "tcp",
		},
		template5G: simulator.Options{
			Mode:          "5g",
			MMEAddr:       "amf:38412",
			TransportMode: "sctp",
		},
	}

	got4G := c.templateForMode("")
	if got4G.Mode != "4g" || got4G.MMEAddr != "mme:36412" || got4G.TransportMode != "tcp" {
		t.Fatalf("default mode picked wrong template: %+v", got4G)
	}

	got5G := c.templateForMode("5g")
	if got5G.Mode != "5g" || got5G.MMEAddr != "amf:38412" || got5G.TransportMode != "sctp" {
		t.Fatalf("5g mode picked wrong template: %+v", got5G)
	}
}
