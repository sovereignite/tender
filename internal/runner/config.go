package runner

import (
	"crypto/rand"
	"fmt"
)

type Config struct {
	Organization string
	Repository   string

	Name     string
	Username string
	Labels   []string
	Group    string

	MemoryMB uint
	CPUs     uint

	NetworkName string
}

func DefaultConfig() Config {
	return Config{
		Name:        GenerateRunnerName(),
		MemoryMB:    4096,
		CPUs:        2,
		NetworkName: "default",
	}
}

func GenerateRunnerName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("runner-%x", b)
}

func (c *Config) Validate() error {
	if c.Organization == "" && c.Repository == "" {
		return fmt.Errorf("organization or repository is required")
	}
	if c.Name == "" {
		c.Name = GenerateRunnerName()
	}
	return nil
}
