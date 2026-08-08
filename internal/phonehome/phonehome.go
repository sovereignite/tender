package phonehome

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mdlayher/vsock"
)

const HostCID = uint32(2)

type Payload struct {
	InstanceID    string `json:"instance_id"`
	Hostname      string `json:"hostname"`
	FQDN          string `json:"fqdn"`
	PubKeyRSA     string `json:"pub_key_rsa"`
	PubKeyECDSA   string `json:"pub_key_ecdsa"`
	PubKeyED25519 string `json:"pub_key_ed25519"`
}

type DialFunc func(contextID, port uint32) (net.Conn, error)

func Collect(instanceID string) (Payload, error) {
	if instanceID == "" {
		return Payload{}, fmt.Errorf("instance ID is required")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return Payload{}, fmt.Errorf("get hostname: %w", err)
	}
	fqdn := hostname
	if canonical, err := net.LookupCNAME(hostname); err == nil {
		fqdn = strings.TrimSuffix(canonical, ".")
	}
	return Payload{
		InstanceID:    instanceID,
		Hostname:      hostname,
		FQDN:          fqdn,
		PubKeyRSA:     readKey("/etc/ssh/ssh_host_rsa_key.pub"),
		PubKeyECDSA:   readKey("/etc/ssh/ssh_host_ecdsa_key.pub"),
		PubKeyED25519: readKey("/etc/ssh/ssh_host_ed25519_key.pub"),
	}, nil
}

func Send(ctx context.Context, port uint32, payload Payload) error {
	return send(ctx, port, payload, func(contextID, port uint32) (net.Conn, error) {
		return vsock.Dial(contextID, port, nil)
	})
}

func send(ctx context.Context, port uint32, payload Payload, dial DialFunc) error {
	if port == 0 {
		return fmt.Errorf("vsock port is required")
	}
	message, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode phone-home payload: %w", err)
	}
	message = append(message, '\n')

	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := dial(HostCID, port)
		if err == nil {
			deadline := time.Now().Add(10 * time.Second)
			_ = conn.SetDeadline(deadline)
			if _, err = conn.Write(message); err == nil {
				response := make([]byte, 3)
				_, err = io.ReadFull(conn, response)
				if err == nil && string(response) == "OK\n" {
					_ = conn.Close()
					return nil
				}
				if err == nil {
					err = fmt.Errorf("invalid response %q", response)
				}
			}
			_ = conn.Close()
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("phone home failed after 10 attempts: %w", lastErr)
}

func readKey(path string) string {
	key, err := os.ReadFile(path)
	if err != nil {
		return "N/A"
	}
	return string(key)
}
