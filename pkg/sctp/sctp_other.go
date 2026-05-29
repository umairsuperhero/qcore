//go:build !linux

package sctp

import "fmt"

func listenSCTP(addr string) (Listener, error) {
	return nil, fmt.Errorf("Native SCTP requires Linux. Use ModeTCP for dev on this platform.")
}

func dialSCTP(addr string) (Association, error) {
	return nil, fmt.Errorf("Native SCTP requires Linux. Use ModeTCP for dev on this platform.")
}
