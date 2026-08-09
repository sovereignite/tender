package cloudinit

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backendfile "github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
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
		"install -m 0755 /mnt/gh-runner-tools/distaff /usr/local/libexec/distaff",
		"distaff --instance-id \"runner\" --port 12345",
		"mount -o ro LABEL=GH_RUNNER_TOOLS /mnt/gh-runner-tools",
		"tar xzf /mnt/gh-runner-tools/actions-runner.tar.gz",
	} {
		if !strings.Contains(userData, expected) {
			t.Errorf("generated user data does not contain %q", expected)
		}
	}
	if strings.Contains(userData, "phone_home:") || strings.Contains(userData, "http://") {
		t.Fatal("generated user data still contains HTTP phone-home configuration")
	}
	if strings.Contains(userData, "api.github.com/repos/actions/runner") || strings.Contains(userData, "actions-runner-linux-x64") {
		t.Fatal("generated user data still downloads the runner")
	}
	if strings.Contains(userData, "--ephemeral") {
		t.Fatal("generated user data configures an ephemeral runner")
	}
}

func TestBuildSeedImage(t *testing.T) {
	seed, err := BuildSeedImage("#cloud-config\nhostname: runner\n", "instance-id: runner\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(seed) == 0 {
		t.Fatal("BuildSeedImage() returned an empty image")
	}
	path := filepath.Join(t.TempDir(), "seed.iso")
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	image, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = image.Close() }()
	filesystem, err := iso9660.Read(backendfile.New(image, true), int64(len(seed)), 0, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(filesystem.Label(), "\x00 ") != "cidata" {
		t.Fatalf("seed label is %q", filesystem.Label())
	}
	for name, expected := range map[string]string{
		"/user-data": "#cloud-config\nhostname: runner\n",
		"/meta-data": "instance-id: runner\n",
	} {
		file, err := filesystem.OpenFile(name, os.O_RDONLY)
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(contents) != expected {
			t.Fatalf("%s is %q", name, contents)
		}
	}
}
