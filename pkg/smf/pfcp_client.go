package smf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/pfcp"
)

var (
	ErrPFCPTimeout = errors.New("smf: pfcp request timed out")
)

// PFCPClient wraps a UDP connection to a UPF and manages PFCP transactions.
type PFCPClient struct {
	localAddr  *net.UDPAddr
	remoteAddr *net.UDPAddr
	conn       *net.UDPConn
	log        logger.Logger
	
	seq atomic.Uint32
	
	mu           sync.Mutex
	transactions map[uint32]chan *pfcp.Message
}

// NewPFCPClient creates a new PFCP client connecting to the given UPF address.
func NewPFCPClient(localIP, upfAddr string, log logger.Logger) (*PFCPClient, error) {
	remote, err := net.ResolveUDPAddr("udp", upfAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve UPF address: %w", err)
	}
	
	local, err := net.ResolveUDPAddr("udp", localIP+":8805") // 8805 is PFCP port
	if err != nil {
		return nil, fmt.Errorf("resolve local PFCP address: %w", err)
	}
	
	conn, err := net.ListenUDP("udp", local)
	if err != nil {
		return nil, fmt.Errorf("listen PFCP UDP: %w", err)
	}
	
	cli := &PFCPClient{
		localAddr:    local,
		remoteAddr:   remote,
		conn:         conn,
		log:          log.WithField("component", "pfcp_client"),
		transactions: make(map[uint32]chan *pfcp.Message),
	}
	
	go cli.readLoop()
	return cli, nil
}

func (c *PFCPClient) readLoop() {
	buf := make([]byte, 2048)
	for {
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			c.log.WithError(err).Error("PFCP read error")
			return
		}
		
		msg, err := pfcp.DecodeMessage(buf[:n])
		if err != nil {
			c.log.WithError(err).Warn("Failed to decode PFCP message")
			continue
		}
		
		c.log.WithFields(map[string]interface{}{
			"type":   msg.Header.MessageType,
			"seq":    msg.Header.SequenceNumber,
			"remote": addr.String(),
		}).Debug("Received PFCP message")
		
		c.mu.Lock()
		ch, ok := c.transactions[msg.Header.SequenceNumber]
		if ok {
			delete(c.transactions, msg.Header.SequenceNumber)
		}
		c.mu.Unlock()
		
		if ok {
			ch <- msg
		} else {
			c.log.WithField("seq", msg.Header.SequenceNumber).Warn("Received PFCP response for unknown transaction")
		}
	}
}

// SendRequest sends a PFCP message and waits for a response matching the sequence number.
func (c *PFCPClient) SendRequest(ctx context.Context, msg *pfcp.Message) (*pfcp.Message, error) {
	// Assign Sequence Number
	seq := c.seq.Add(1)
	msg.Header.SequenceNumber = seq
	
	ch := make(chan *pfcp.Message, 1)
	c.mu.Lock()
	c.transactions[seq] = ch
	c.mu.Unlock()
	
	defer func() {
		c.mu.Lock()
		delete(c.transactions, seq)
		c.mu.Unlock()
	}()
	
	b := msg.Encode()
	_, err := c.conn.WriteToUDP(b, c.remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("write udp: %w", err)
	}
	
	c.log.WithFields(map[string]interface{}{
		"type": msg.Header.MessageType,
		"seq":  seq,
	}).Debug("Sent PFCP message")
	
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(3 * time.Second):
		return nil, ErrPFCPTimeout
	case resp := <-ch:
		return resp, nil
	}
}

// Close shuts down the PFCP client.
func (c *PFCPClient) Close() error {
	return c.conn.Close()
}
