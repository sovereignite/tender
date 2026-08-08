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
