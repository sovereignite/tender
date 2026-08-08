package libvirt

import "testing"

func TestNetworkGateway(t *testing.T) {
	networkXML := `<network>
  <name>default</name>
  <ip address="192.168.122.1" netmask="255.255.255.0"/>
</network>`

	gateway, err := networkGateway(networkXML)
	if err != nil {
		t.Fatal(err)
	}
	if gateway != "192.168.122.1" {
		t.Fatalf("networkGateway() = %q", gateway)
	}
}

func TestNetworkGatewayRejectsIPv6OnlyNetwork(t *testing.T) {
	networkXML := `<network>
  <name>default</name>
  <ip family="ipv6" address="fd00::1" prefix="64"/>
</network>`

	if _, err := networkGateway(networkXML); err == nil {
		t.Fatal("networkGateway() error = nil, want an error")
	}
}
