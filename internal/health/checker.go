package health

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/sovereignite/gh-workers/internal/libvirt"
	"github.com/sovereignite/gh-workers/internal/runner"
)

// Status represents the health status of a runner.
type Status struct {
	Name      string
	Healthy   bool
	State     string
	IP        string
	LastCheck time.Time
	Error     string
}

// Checker monitors runner health and handles auto-recovery.
type Checker struct {
	client    *libvirt.Client
	manager   *runner.Manager
	interval  time.Duration
	timeout   time.Duration
	mu        sync.RWMutex
	statuses  map[string]*Status
	stopCh    chan struct{}
}

// NewChecker creates a new health checker.
func NewChecker(client *libvirt.Client, interval, timeout time.Duration) *Checker {
	return &Checker{
		client:   client,
		manager:  runner.NewManager(client),
		interval: interval,
		timeout:  timeout,
		statuses: make(map[string]*Status),
		stopCh:   make(chan struct{}),
	}
}

// Start begins the health checking loop.
func (c *Checker) Start(ctx context.Context) {
	go c.checkLoop(ctx)
}

// Stop stops the health checker.
func (c *Checker) Stop() {
	close(c.stopCh)
}

// GetStatus returns the health status of a runner.
func (c *Checker) GetStatus(name string) *Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statuses[name]
}

// GetAllStatuses returns the health status of all runners.
func (c *Checker) GetAllStatuses() map[string]*Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]*Status)
	for k, v := range c.statuses {
		result[k] = v
	}
	return result
}

func (c *Checker) checkLoop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.checkAll(ctx)
		}
	}
}

func (c *Checker) checkAll(ctx context.Context) {
	domains, err := c.manager.List()
	if err != nil {
		log.Printf("Failed to list domains: %v", err)
		return
	}

	for _, domain := range domains {
		go c.checkDomain(ctx, domain)
	}
}

func (c *Checker) checkDomain(ctx context.Context, domain libvirt.DomainStatus) {
	status := &Status{
		Name:      domain.Name,
		State:     domain.State,
		LastCheck: time.Now(),
	}

	// Check if domain is running
	if domain.State != "running" {
		status.Healthy = false
		status.Error = fmt.Sprintf("domain is %s", domain.State)

		// Attempt recovery if domain is shutoff
		if domain.State == "shutoff" {
			c.recoverDomain(ctx, domain.Name)
		}
	} else {
		// Domain is running, check if it has an IP
		ip, err := c.client.WaitForDomainIP(domain.Name, c.timeout)
		if err != nil {
			status.Healthy = false
			status.Error = fmt.Sprintf("no IP: %v", err)
		} else {
			status.Healthy = true
			status.IP = ip
		}
	}

	c.mu.Lock()
	c.statuses[domain.Name] = status
	c.mu.Unlock()
}

func (c *Checker) recoverDomain(ctx context.Context, name string) {
	log.Printf("Attempting to recover runner %s", name)

	// Try to start the domain
	if err := c.manager.Start(name); err != nil {
		log.Printf("Failed to recover runner %s: %v", name, err)
		return
	}

	log.Printf("Successfully recovered runner %s", name)
}

// HealthCheck performs a one-time health check on all runners.
func (c *Checker) HealthCheck(ctx context.Context) (map[string]*Status, error) {
	domains, err := c.manager.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	var wg sync.WaitGroup
	for _, domain := range domains {
		wg.Add(1)
		go func(d libvirt.DomainStatus) {
			defer wg.Done()
			c.checkDomain(ctx, d)
		}(domain)
	}
	wg.Wait()

	return c.GetAllStatuses(), nil
}
