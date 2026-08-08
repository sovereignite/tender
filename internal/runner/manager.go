package runner

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sovereignite/gh-workers/internal/cloudinit"
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
	phoneHome *phoneHomeServer
	baseImage string
}

func NewManager(client *libvirt.Client) *Manager {
	return &Manager{
		client:    client,
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

func (m *Manager) startPhoneHomeServer(networkName string) error {
	if m.phoneHome.server != nil {
		return nil
	}

	gateway, err := m.client.NetworkGateway(networkName)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(gateway, "0"))
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

func (m *Manager) EnsureInfrastructure() error {
	if err := m.client.EnsureImagesPool(); err != nil {
		return fmt.Errorf("failed to ensure images pool: %w", err)
	}

	selectedImage, err := images.SelectImage(images.Selector{})
	if err != nil {
		return fmt.Errorf("failed to select base image: %w", err)
	}

	baseImage, err := m.client.CacheImage(*selectedImage)
	if err != nil {
		return fmt.Errorf("failed to cache base image: %w", err)
	}
	m.baseImage = baseImage

	return nil
}

func (m *Manager) Create(cfg Config) error {
	return m.create(cfg, "")
}

func (m *Manager) create(cfg Config, seedPath string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	diskName := cfg.Name + ".qcow2"
	if m.baseImage == "" {
		return fmt.Errorf("runner infrastructure is not initialized")
	}
	diskPath, err := m.client.CloneVolume(diskName, m.baseImage)
	if err != nil {
		return fmt.Errorf("failed to clone volume: %w", err)
	}

	domCfg := libvirt.DomainConfig{
		Name:        cfg.Name,
		MemoryMB:    cfg.MemoryMB,
		CPUs:        cfg.CPUs,
		DiskPath:    diskPath,
		SeedPath:    seedPath,
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

	if token == "" {
		return fmt.Errorf("runner registration token is required")
	}

	if err := m.startPhoneHomeServer(cfg.NetworkName); err != nil {
		return fmt.Errorf("failed to start phone-home server: %w", err)
	}

	cloudCfg := cloudinit.DefaultConfig(cfg.Name, cfg.Organization, token)
	cloudCfg.Labels = cfg.Labels
	cloudCfg.Group = cfg.Group
	if cfg.Username != "" {
		cloudCfg.Username = cfg.Username
	}
	cloudCfg.PhoneHomeURL = fmt.Sprintf("http://%s/phone-home", m.GetPhoneHomeAddress())

	userData, err := cloudinit.GenerateUserData(cloudCfg)
	if err != nil {
		return fmt.Errorf("failed to generate user data: %w", err)
	}

	metaData := cloudinit.GenerateMetaConfig(cloudCfg)

	seed, err := cloudinit.BuildSeedImage(userData, metaData)
	if err != nil {
		return err
	}
	seedPath, err := m.client.CreateSeedVolume(cfg.Name+"-seed.img", seed)
	if err != nil {
		return fmt.Errorf("failed to create cloud-init seed volume: %w", err)
	}
	return m.create(cfg, seedPath)
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
	if err := m.client.DeleteVolume(diskName); err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}
	seedName := name + "-seed.img"
	if err := m.client.DeleteVolume(seedName); err != nil {
		return fmt.Errorf("failed to delete seed volume: %w", err)
	}

	return nil
}

func (m *Manager) Status(name string) (*libvirt.DomainStatus, error) {
	return m.client.GetDomainStatus(name)
}

func (m *Manager) DiskInfo(name string) (*libvirt.VolumeInfo, error) {
	return m.client.GetVolumeInfo(name + ".qcow2")
}

func (m *Manager) Console(name string, output io.Writer) error {
	return m.client.OpenConsole(name, output)
}

func (m *Manager) List() ([]libvirt.DomainStatus, error) {
	return m.client.ListDomains()
}

func (m *Manager) WaitForReady(name string, timeout time.Duration) (string, error) {
	return m.client.WaitForDomainIP(name, timeout)
}
