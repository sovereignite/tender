package cloudinit

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGenerateUserDataUsesSSHImportArray(t *testing.T) {
	config := DefaultConfig("runner", "sovereignite", "token")
	config.Username = "opsroller"
	userData, err := GenerateUserData(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(userData, "ssh_import_id:\n      - gh:opsroller") {
		t.Fatal("ssh_import_id is not rendered as an array")
	}
}

func TestGenerateUserDataUsesVsockPhoneHome(t *testing.T) {
	config := DefaultConfig("runner", "sovereignite", "token")
	config.PhoneHomePort = 12345
	userData, err := GenerateUserData(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"path: /usr/local/libexec/gh-runner-phone-home.py",
		"socket.AF_VSOCK",
		"socket.VMADDR_CID_HOST",
		`"instance_id"`,
		`"hostname"`,
		`"fqdn"`,
		`"pub_key_rsa"`,
		`"pub_key_ecdsa"`,
		`"pub_key_ed25519"`,
		"gh-runner-phone-home.py \"runner\" 12345",
	} {
		if !strings.Contains(userData, expected) {
			t.Errorf("generated user data does not contain %q", expected)
		}
	}
	if strings.Contains(userData, "phone_home:") || strings.Contains(userData, "http://") {
		t.Fatal("generated user data still contains HTTP phone-home configuration")
	}
}

func TestBuildSeedImage(t *testing.T) {
	if _, err := exec.LookPath("cloud-localds"); err != nil {
		t.Skip("cloud-localds is not installed")
	}

	seed, err := BuildSeedImage("#cloud-config\nhostname: runner\n", "instance-id: runner\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(seed) == 0 {
		t.Fatal("BuildSeedImage() returned an empty image")
	}
}
