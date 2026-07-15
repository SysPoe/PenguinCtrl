package remote

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type transport interface {
	Send(context.Context, string, int, []byte) error
	SendAcknowledged(context.Context, string, int, string, []byte) error
	Probe(context.Context, string, int) error
}

type networkTransport struct{ dialer net.Dialer }

func (t *networkTransport) Send(ctx context.Context, host string, port int, payload []byte) (err error) {
	conn, err := t.dialer.DialContext(ctx, "udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	}
	_, err = conn.Write(payload)
	return err
}

func (t *networkTransport) SendAcknowledged(ctx context.Context, host string, port int, id string, payload []byte) (err error) {
	conn, err := t.dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	request := struct {
		ID      string `json:"id"`
		Payload string `json:"payload"`
	}{ID: id, Payload: base64.StdEncoding.EncodeToString(payload)}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return err
	}
	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(response) != "ACK "+id {
		return fmt.Errorf("unexpected acknowledgement %q", strings.TrimSpace(response))
	}
	return nil
}

func (t *networkTransport) Probe(ctx context.Context, host string, port int) (err error) {
	conn, err := t.dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return conn.Close()
}
