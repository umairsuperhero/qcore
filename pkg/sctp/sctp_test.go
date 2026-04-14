package sctp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPRoundTrip(t *testing.T) {
	// Start a TCP listener
	ln, err := Listen(ModeTCP, "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	addr := ln.Addr().String()

	// Accept in background
	done := make(chan struct{})
	var serverAssoc Association
	var serverErr error
	go func() {
		serverAssoc, serverErr = ln.Accept()
		close(done)
	}()

	// Dial from client
	client, err := Dial(ModeTCP, addr)
	require.NoError(t, err)
	defer client.Close()

	<-done
	require.NoError(t, serverErr)
	defer serverAssoc.Close()

	t.Run("client to server", func(t *testing.T) {
		msg := []byte("hello from eNodeB")
		err := client.Write(msg, 0)
		require.NoError(t, err)

		data, streamID, err := serverAssoc.Read()
		require.NoError(t, err)
		assert.Equal(t, msg, data)
		assert.Equal(t, uint16(0), streamID)
	})

	t.Run("server to client", func(t *testing.T) {
		msg := []byte("hello from MME")
		err := serverAssoc.Write(msg, 1)
		require.NoError(t, err)

		data, streamID, err := client.Read()
		require.NoError(t, err)
		assert.Equal(t, msg, data)
		assert.Equal(t, uint16(1), streamID)
	})

	t.Run("stream IDs preserved", func(t *testing.T) {
		for _, sid := range []uint16{0, 1, 42, 65535} {
			err := client.Write([]byte("test"), sid)
			require.NoError(t, err)

			_, gotSID, err := serverAssoc.Read()
			require.NoError(t, err)
			assert.Equal(t, sid, gotSID)
		}
	})

	t.Run("empty message", func(t *testing.T) {
		err := client.Write([]byte{}, 0)
		require.NoError(t, err)

		data, _, err := serverAssoc.Read()
		require.NoError(t, err)
		assert.Empty(t, data)
	})

	t.Run("large message", func(t *testing.T) {
		msg := make([]byte, 4096)
		for i := range msg {
			msg[i] = byte(i % 256)
		}
		err := client.Write(msg, 3)
		require.NoError(t, err)

		data, streamID, err := serverAssoc.Read()
		require.NoError(t, err)
		assert.Equal(t, msg, data)
		assert.Equal(t, uint16(3), streamID)
	})
}

func TestListenModes(t *testing.T) {
	t.Run("tcp mode", func(t *testing.T) {
		ln, err := Listen(ModeTCP, "127.0.0.1:0")
		require.NoError(t, err)
		ln.Close()
	})

	t.Run("empty mode defaults to tcp", func(t *testing.T) {
		ln, err := Listen("", "127.0.0.1:0")
		require.NoError(t, err)
		ln.Close()
	})

	t.Run("unsupported mode", func(t *testing.T) {
		_, err := Listen("quic", "127.0.0.1:0")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})
}
