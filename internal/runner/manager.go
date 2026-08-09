package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/mdlayher/vsock"
	"github.com/sovereignite/shuttle/internal/cloudinit"
	"github.com/sovereignite/shuttle/internal/images"
	"github.com/sovereignite/shuttle/internal/libvirt"
	"github.com/sovereignite/shuttle/internal/phonehome"
)

type PhoneHomeEvent struct {
	CID           uint32
	Hostname      string
	FQDN          string
	PubKeyRSA     string
	PubKeyECDSA   string
	PubKeyED25519 string
	Ready         bool
}

type phoneHomeServer struct {
	events map[string]*PhoneHomeEvent
	mu     sync.RWMutex
	server *vsock.Listener
	port   uint32
}

type Manager struct {
	client    *libvirt.Client
	phoneHome *phoneHomeServer
	baseImage string
	toolsPath string
}

func NewManager(client *libvirt.Client) *Manager {
	return &Manager{
		client:    client,
		phoneHome: &phoneHomeServer{events: make(map[string]*PhoneHomeEvent)},
	}
}

func (m *Manager) GetPhoneHomePort() uint32 {
	return m.phoneHome.port
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

	listener, err := vsock.Listen(0, nil)
	if err != nil {
		return fmt.Errorf("failed to listen on vsock: %w", err)
	}
	addr, ok := listener.Addr().(*vsock.Addr)
	if !ok {
		_ = listener.Close()
		return fmt.Errorf("unexpected vsock listener address %T", listener.Addr())
	}
	m.phoneHome.server = listener
	m.phoneHome.port = addr.Port

	go m.acceptPhoneHome()

	return nil
}

func (m *Manager) acceptPhoneHome() {
	for {
		conn, err := m.phoneHome.server.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = conn.Close() }()

			var payload phonehome.Payload
			if err := json.NewDecoder(conn).Decode(&payload); err != nil || payload.InstanceID == "" {
				return
			}

			var cid uint32
			if peer, ok := conn.RemoteAddr().(*vsock.Addr); ok {
				cid = peer.ContextID
			}
			m.phoneHome.mu.Lock()
			m.phoneHome.events[payload.InstanceID] = &PhoneHomeEvent{
				CID:           cid,
				Hostname:      payload.Hostname,
				FQDN:          payload.FQDN,
				PubKeyRSA:     payload.PubKeyRSA,
				PubKeyECDSA:   payload.PubKeyECDSA,
				PubKeyED25519: payload.PubKeyED25519,
				Ready:         true,
			}
			m.phoneHome.mu.Unlock()
			_, _ = conn.Write([]byte("OK\n"))
		}()
	}
}

func (m *Manager) EnsureInfrastructure(callbackPath string) error {
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

	runnerRelease, err := images.LatestRunnerRelease()
	if err != nil {
		return fmt.Errorf("failed to select GitHub Actions runner release: %w", err)
	}
	toolsPath, err := m.client.CacheRunnerTools(runnerRelease, callbackPath)
	if err != nil {
		return fmt.Errorf("failed to cache GitHub Actions runner tools: %w", err)
	}
	m.toolsPath = toolsPath

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
	if m.toolsPath == "" {
		return fmt.Errorf("runner tools are not initialized")
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
		ToolsPath:   m.toolsPath,
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

	if err := m.startPhoneHomeServer(); err != nil {
		return fmt.Errorf("failed to start phone-home server: %w", err)
	}

	cloudCfg := cloudinit.DefaultConfig(cfg.Name, cfg.Organization, token)
	cloudCfg.Labels = cfg.Labels
	cloudCfg.Group = cfg.Group
	if cfg.Username != "" {
		cloudCfg.Username = cfg.Username
	}
	cloudCfg.PhoneHomePort = m.GetPhoneHomePort()

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
