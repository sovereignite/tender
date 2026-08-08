package phonehome

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
)

func TestSend(t *testing.T) {
	payload := Payload{InstanceID: "runner-1", Hostname: "runner-1", FQDN: "runner-1.example"}
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()

	dialed := make(chan struct{})
	go func() {
		defer close(dialed)
		var received Payload
		if err := json.NewDecoder(bufio.NewReader(server)).Decode(&received); err != nil {
			t.Error(err)
			return
		}
		if received != payload {
			t.Errorf("payload = %+v, want %+v", received, payload)
		}
		_, _ = server.Write([]byte("OK\n"))
	}()

	err := send(context.Background(), 12345, payload, func(cid, port uint32) (net.Conn, error) {
		if cid != HostCID || port != 12345 {
			t.Fatalf("dial(%d, %d)", cid, port)
		}
		return client, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-dialed
}
