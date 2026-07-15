package redundancy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	protocolVersion     = 1
	heartbeatBufferSize = 64 << 10
)

type heartbeat struct {
	Version      int         `json:"version"`
	NodeID       string      `json:"nodeId"`
	Role         Role        `json:"role"`
	BootID       string      `json:"bootId"`
	Sequence     uint64      `json:"sequence"`
	SentUnixNano int64       `json:"sentUnixNano"`
	Active       bool        `json:"active"`
	Fingerprint  Fingerprint `json:"fingerprint"`
	InterlockID  string      `json:"interlockId"`
	Signature    string      `json:"signature,omitempty"`
}

type peerTransport struct {
	conn        *net.UDPConn
	peerAddress *net.UDPAddr
	interval    time.Duration
	sharedKey   string
	next        func(time.Time) heartbeat
	receive     func(heartbeat, time.Time)
	fail        func(string)
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func openPeerTransport(config Config, next func(time.Time) heartbeat, receive func(heartbeat, time.Time), fail func(string)) (*peerTransport, error) {
	listenAddress, err := net.ResolveUDPAddr("udp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("resolve redundancy listen address: %w", err)
	}
	peerAddress, err := net.ResolveUDPAddr("udp", config.PeerAddress)
	if err != nil {
		return nil, fmt.Errorf("resolve redundancy peer address: %w", err)
	}
	conn, err := net.ListenUDP("udp", listenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen for redundancy heartbeats: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	transport := &peerTransport{
		conn: conn, peerAddress: peerAddress, interval: config.HeartbeatInterval, sharedKey: config.SharedKey,
		next: next, receive: receive, fail: fail, cancel: cancel,
	}
	transport.wg.Add(1)
	go transport.run(ctx)
	return transport, nil
}

func (t *peerTransport) close() {
	if t == nil {
		return
	}
	t.cancel()
	_ = t.conn.Close()
	t.wg.Wait()
}

func (t *peerTransport) run(ctx context.Context) {
	defer t.wg.Done()
	nextHeartbeat := time.Now()
	buffer := make([]byte, heartbeatBufferSize)
	for {
		if wait := time.Until(nextHeartbeat); wait <= 0 {
			t.send(time.Now())
			nextHeartbeat = time.Now().Add(t.interval)
		}
		_ = t.conn.SetReadDeadline(nextHeartbeat)
		n, address, err := t.conn.ReadFromUDP(buffer)
		if err == nil {
			t.handle(address, buffer[:n])
			continue
		}
		if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
			return
		}
		if netError, ok := err.(net.Error); ok && netError.Timeout() {
			continue
		}
		t.fail("receive redundancy heartbeat: " + err.Error())
	}
}

func (t *peerTransport) send(now time.Time) {
	message := t.next(now)
	signature, err := signHeartbeat(message, t.sharedKey)
	if err == nil {
		message.Signature = signature
		var raw []byte
		raw, err = json.Marshal(message)
		if err == nil {
			_, err = t.conn.WriteToUDP(raw, t.peerAddress)
		}
	}
	if err != nil {
		t.fail("send redundancy heartbeat: " + err.Error())
	}
}

func (t *peerTransport) handle(address *net.UDPAddr, raw []byte) {
	if !sameUDPAddress(address, t.peerAddress) {
		return
	}
	var message heartbeat
	if err := json.Unmarshal(raw, &message); err != nil || message.Version != protocolVersion {
		return
	}
	if !verifyHeartbeat(message, t.sharedKey) {
		return
	}
	t.receive(message, time.Now())
}

func signHeartbeat(message heartbeat, key string) (string, error) {
	message.Signature = ""
	raw, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func verifyHeartbeat(message heartbeat, key string) bool {
	expectedSignature, err := signHeartbeat(message, key)
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(expectedSignature)
	if err != nil {
		return false
	}
	actual, err := hex.DecodeString(message.Signature)
	return err == nil && hmac.Equal(actual, expected)
}

func sameUDPAddress(actual, expected *net.UDPAddr) bool {
	if actual == nil || expected == nil || actual.Port != expected.Port {
		return false
	}
	actualIP, expectedIP := actual.IP.To16(), expected.IP.To16()
	return actualIP != nil && expectedIP != nil && actualIP.Equal(expectedIP)
}
