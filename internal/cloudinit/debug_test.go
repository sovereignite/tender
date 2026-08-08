package cloudinit

import (
	"fmt"
	"testing"
)

func TestDebugConfigShLine(t *testing.T) {
	out, err := GenerateUserData(Config{
		RunnerName:    "test-runner",
		Organization:  "sovereignite",
		Token:         "test-token",
		Hostname:      "test-runner",
		Username:      "ubuntu",
		PhoneHomePort: 9999,
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(out)
}
