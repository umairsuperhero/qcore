//go:build linux

package sctp

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAddrResolvesHostnames(t *testing.T) {
	sa, err := parseAddr("localhost:38412")
	require.NoError(t, err)

	addr, ok := sa.(*syscall.SockaddrInet4)
	require.True(t, ok)
	require.Equal(t, 38412, addr.Port)
	require.NotEqual(t, [4]byte{}, addr.Addr)
}
