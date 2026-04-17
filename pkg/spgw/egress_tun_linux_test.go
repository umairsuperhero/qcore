//go:build linux

package spgw

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"golang.org/x/sys/unix"
)

// TestTUNEgress_RealKernel exercises the real /dev/net/tun code path.
//
// Requires CAP_NET_ADMIN. In CI this test runs under `sudo -E go test`
// via the linux-integration job. Locally it will Skip if permissions
// are missing, so a regular `go test ./...` on Linux stays green.
func TestTUNEgress_RealKernel(t *testing.T) {
	// Use a pid-qualified device name so concurrent test runs don't collide
	// and so a stale device from a crashed earlier run doesn't shadow this one.
	devName := fmt.Sprintf("qctst%d", os.Getpid()%100000)

	log := logger.New("debug", "text")

	eg, err := NewTUNEgress(log, devName, 1400)
	if err != nil {
		if isPermissionError(err) {
			t.Skipf("TUN smoke requires CAP_NET_ADMIN; re-run under sudo to exercise this path. Got: %v", err)
		}
		t.Fatalf("NewTUNEgress(%q): %v", devName, err)
	}

	// Interface should now be visible to the kernel.
	iface, err := net.InterfaceByName(devName)
	if err != nil {
		_ = eg.Close()
		t.Fatalf("interface %q not visible after open: %v", devName, err)
	}
	if iface.MTU <= 0 {
		t.Logf("note: kernel MTU=%d (TUNSETIFF does not set MTU; that's fine)", iface.MTU)
	}

	// A TUN device returns EIO on writes until the interface is UP — the
	// kernel treats writes as "packet arrived on this NIC" and the NIC must
	// be administratively up to ingest frames. Prod operators bring it up
	// with `ip link set qcore0 up`; we do the same here so the Send probe
	// actually exercises the write path.
	if out, err := exec.Command("ip", "link", "set", devName, "up").CombinedOutput(); err != nil {
		t.Logf("ip link set %s up: %v — skipping Send probe (iproute2 missing?): %s",
			devName, err, string(out))
	} else {
		// Uplink write: kernel accepts bytes into the interface's ingress queue.
		// No address is configured, so routing will drop the packet — but Write()
		// succeeds based on the fd being writable and the device being UP, which
		// is what we're checking.
		pkt := buildMinimalIPv4Packet(t)
		if err := eg.Send(pkt); err != nil {
			_ = eg.Close()
			t.Fatalf("Send on live TUN (after ip link up): %v", err)
		}
	}

	// Brief pause so the read goroutine (readLoop) gets scheduled at least
	// once — Close must cleanly unblock it.
	time.Sleep(20 * time.Millisecond)

	if err := eg.Close(); err != nil {
		// On Linux, closing the *os.File that wraps the tun fd can report
		// EBADF for the second close path (the goroutine's interrupted Read).
		// That's benign — the device teardown still happened.
		t.Logf("Close returned (benign on Linux TUN): %v", err)
	}

	// Closing the last fd on a TUN device makes the kernel tear it down,
	// so the interface should disappear. We give it a tick.
	if !waitForInterfaceGone(devName, 500*time.Millisecond) {
		t.Errorf("interface %q still present after Close + 500ms", devName)
	}
}

// TestTUNEgress_NameTooLong checks the validation path without needing root.
func TestTUNEgress_NameTooLong(t *testing.T) {
	log := logger.New("error", "text")
	_, err := NewTUNEgress(log, "thisnameismuchtoolongforifnamsiz", 1400)
	if err == nil {
		t.Fatal("expected error for oversized device name, got nil")
	}
}

// buildMinimalIPv4Packet crafts a 20-byte IPv4-only frame (no payload) addressed
// to 10.99.0.2. Protocol field is 0; the packet will be dropped by the kernel
// after ingress, but the TUN write path doesn't care about routing outcome.
func buildMinimalIPv4Packet(t *testing.T) []byte {
	t.Helper()
	hdr := make([]byte, 20)
	hdr[0] = 0x45 // version=4, IHL=5
	hdr[1] = 0x00 // DSCP/ECN
	binary.BigEndian.PutUint16(hdr[2:4], 20)
	binary.BigEndian.PutUint16(hdr[4:6], 0)
	binary.BigEndian.PutUint16(hdr[6:8], 0)
	hdr[8] = 64
	hdr[9] = 0
	binary.BigEndian.PutUint16(hdr[10:12], 0)
	copy(hdr[12:16], []byte{10, 99, 0, 1})
	copy(hdr[16:20], []byte{10, 99, 0, 2})
	binary.BigEndian.PutUint16(hdr[10:12], ipv4Checksum(hdr))
	return hdr
}

func ipv4Checksum(hdr []byte) uint16 {
	var sum uint32
	for i := 0; i < len(hdr); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(hdr[i : i+2]))
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	var errno unix.Errno
	if errors.As(err, &errno) {
		return errno == unix.EPERM || errno == unix.EACCES
	}
	return false
}

func waitForInterfaceGone(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := net.InterfaceByName(name); err != nil {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	_, err := net.InterfaceByName(name)
	return err != nil
}
