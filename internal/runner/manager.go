package runner

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sovereignite/gh-workers/internal/cloudinit"
	"github.com/sovereignite/gh-workers/internal/github"
	"github.com/sovereignite/gh-workers/internal/images"
	"github.com/sovereignite/gh-workers/internal/libvirt"
)

type PhoneHomeEvent struct {
	IP       string
	Hostname string
	Ready    bool
}

type phoneHomeServer struct {
	events map[string]*PhoneHomeEvent
	mu     sync.RWMutex
	server *http.Server
	addr   string
}

type Manager struct {
	client    *libvirt.Client
	github    *github.App
	phoneHome *phoneHomeServer
}

func NewManager(client *libvirt.Client) *Manager {
	return &Manager{
		client:    client,
		phoneHome: &phoneHomeServer{events: make(map[string]*PhoneHomeEvent)},
	}
}

func NewManagerWithGitHub(client *libvirt.Client, app *github.App) *Manager {
	return &Manager{
		client:    client,
		github:    app,
		phoneHome: &phoneHomeServer{events: make(map[string]*PhoneHomeEvent)},
	}
}

func (m *Manager) GetPhoneHomeAddress() string {
	return m.phoneHome.addr
}

func (m *Manager) WaitForPhoneHome(name string, timeout time.Duration) (*PhoneHomeEvent, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.phoneHome.mu.RLock()
		ev, ok := m.phoneHome.events[name]
		m.phoneHome.mu.RUnlock()
		if ok && ev.Ready {
			return ev, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timeout waiting for phone home from %s", name)
}

func (m *Manager) startPhoneHomeServer() error {
	if m.phoneHome.server != nil {
		return nil
	}

	listener, err := net.Listen("tcp", "192.168.122.1:0")
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	m.phoneHome.addr = listener.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/phone-home", func(w http.ResponseWriter, r *http.Request) {
		instanceID := r.FormValue("instance-id")
		hostname := r.FormValue("hostname")
		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip, _, _ = net.SplitHostPort(r.RemoteAddr)
		}

		m.phoneHome.mu.Lock()
		if _, exists := m.phoneHome.events[instanceID]; !exists {
			m.phoneHome.events[instanceID] = &PhoneHomeEvent{IP: ip}
		}
		m.phoneHome.events[instanceID].Hostname = hostname
		m.phoneHome.events[instanceID].Ready = true
		m.phoneHome.mu.Unlock()

		_, _ = fmt.Fprintf(w, "OK")
	})

	m.phoneHome.server = &http.Server{Handler: mux}

	go func() { _ = m.phoneHome.server.Serve(listener) }()

	return nil
}

func (m *Manager) EnsureInfrastructure(poolName string) error {
	// Create images pool for base images if it doesn't exist
	if !m.client.PoolExists("images") {
		if err := m.client.CreatePool(libvirt.StorageConfig{Name: "images"}); err != nil {
			return fmt.Errorf("failed to create images pool: %w", err)
		}
	}

	// Create runner pool if it doesn't exist
	if !m.client.PoolExists(poolName) {
		if err := m.client.CreatePool(libvirt.StorageConfig{Name: poolName}); err != nil {
			return fmt.Errorf("failed to create pool: %w", err)
		}
	}

	// Get latest Ubuntu LTS image
	latestLTS, err := images.GetLatestLTS()
	if err != nil {
		return fmt.Errorf("failed to get latest LTS image: %w", err)
	}

	if err := m.client.DownloadBaseImage("images", latestLTS.Name); err != nil {
		return fmt.Errorf("failed to download base image: %w", err)
	}

	return nil
}

func (m *Manager) Create(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	diskName := cfg.Name + ".qcow2"
	diskPath, err := m.client.CloneVolume(cfg.PoolName, diskName)
	if err != nil {
		return fmt.Errorf("failed to clone volume: %w", err)
	}

	domCfg := libvirt.DomainConfig{
		Name:        cfg.Name,
		MemoryMB:    cfg.MemoryMB,
		CPUs:        cfg.CPUs,
		DiskPath:    diskPath,
		NetworkName: cfg.NetworkName,
	}

	if err := m.client.CreateDomain(domCfg); err != nil {
		return fmt.Errorf("failed to create domain: %w", err)
	}

	return nil
}

func (m *Manager) CreateWithCloudInit(cfg Config, token string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if token == "" && m.github != nil {
		t, err := m.github.GetRunnerRegistrationToken()
		if err != nil {
			return fmt.Errorf("failed to get runner token: %w", err)
		}
		token = t.Token
	}

	if token == "" {
		return fmt.Errorf("token is required (use --token or GitHub App)")
	}

	if err := m.startPhoneHomeServer(); err != nil {
		return fmt.Errorf("failed to start phone-home server: %w", err)
	}

	cloudCfg := cloudinit.DefaultConfig(cfg.Name, cfg.Organization, token)
	cloudCfg.Labels = cfg.Labels
	cloudCfg.Group = cfg.Group
	cloudCfg.PhoneHomeURL = fmt.Sprintf("http://%s/phone-home", m.GetPhoneHomeAddress())

	seedDir := filepath.Join("/tmp", cfg.Name, "seed")
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return fmt.Errorf("failed to create seed directory: %w", err)
	}

	userData, err := cloudinit.GenerateUserData(cloudCfg)
	if err != nil {
		return fmt.Errorf("failed to generate user data: %w", err)
	}

	metaData := cloudinit.GenerateMetaConfig(cloudCfg)

	if err := os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(userData), 0644); err != nil {
		return fmt.Errorf("failed to write user-data: %w", err)
	}

	if err := os.WriteFile(filepath.Join(seedDir, "meta-data"), []byte(metaData), 0644); err != nil {
		return fmt.Errorf("failed to write meta-data: %w", err)
	}

	return m.Create(cfg)
}

func (m *Manager) Start(name string) error {
	return m.client.StartDomain(name)
}

func (m *Manager) Stop(name string) error {
	return m.client.StopDomain(name)
}

func (m *Manager) ForceStop(name string) error {
	return m.client.ForceStopDomain(name)
}

func (m *Manager) Destroy(name string) error {
	if err := m.client.DestroyDomain(name); err != nil {
		return fmt.Errorf("failed to destroy domain: %w", err)
	}

	diskName := name + ".qcow2"
	if err := m.client.DeleteVolume("Downloads", diskName); err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	return nil
}

func (m *Manager) Status(name string) (*libvirt.DomainStatus, error) {
	return m.client.GetDomainStatus(name)
}

func (m *Manager) List() ([]libvirt.DomainStatus, error) {
	return m.client.ListDomains()
}

func (m *Manager) WaitForReady(name string, timeout time.Duration) (string, error) {
	return m.client.WaitForDomainIP(name, timeout)
}
