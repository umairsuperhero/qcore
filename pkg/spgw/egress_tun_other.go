//go:build !linux

package spgw

import (
	"fmt"
	"runtime"

	"github.com/qcore-project/qcore/pkg/logger"
)

// NewTUNEgress is a stub on non-Linux platforms. The Linux implementation
// uses /dev/net/tun + TUNSETIFF, which has no portable equivalent. Operators
// running on macOS or Windows should keep egress="log" for development and
// run the SPGW inside a Linux container (or VM) for real packet forwarding.
func NewTUNEgress(_ logger.Logger, _ string, _ int) (Egress, error) {
	return nil, fmt.Errorf("tun egress not supported on %s — use egress=log for dev or run inside Linux", runtime.GOOS)
}
