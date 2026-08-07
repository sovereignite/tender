package runner

import "fmt"

// Config holds the configuration for a GitHub Actions runner.
type Config struct {
	// GitHub organization or repository
	Organization string
	Repository   string

	// Runner configuration
	Name   string
	Labels []string
	Group  string

	// VM configuration
	MemoryMB uint
	CPUs     uint

	// Network configuration
	NetworkName string
	Subnet      string

	// Storage configuration
	PoolName string
}

// DefaultConfig returns the default runner configuration.
func DefaultConfig(name string) Config {
	return Config{
		Name:        name,
		Labels:      []string{"self-hosted", "linux", "x64"},
		MemoryMB:    4096,
		CPUs:        2,
		NetworkName: "gh-runners",
		Subnet:      "192.168.122",
		PoolName:    "gh-runners",
	}
}

// Validate validates the runner configuration.
func (c *Config) Validate() error {
	if c.Organization == "" && c.Repository == "" {
		return fmt.Errorf("organization or repository is required")
	}
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}
