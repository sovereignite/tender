package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sovereignite/tender/internal/config"
	"github.com/sovereignite/tender/internal/github"
	"github.com/sovereignite/tender/internal/health"
	"github.com/sovereignite/tender/internal/images"
	"github.com/sovereignite/tender/internal/libvirt"
	"github.com/sovereignite/tender/internal/logging"
	"github.com/sovereignite/tender/internal/runner"
	"github.com/spf13/cobra"
)

func imageDate(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format("2006-01-02")
}

func imageSize(value uint64) string {
	if value == 0 {
		return "-"
	}
	const mib = 1024 * 1024
	return strconv.FormatUint((value+mib-1)/mib, 10) + " MiB"
}

func main() {
	cfg := config.DefaultConfig()
	var logger *logging.Logger

	rootCmd := &cobra.Command{
		Use:   "shuttle",
		Short: "Build installable Linux systems",
		Long:  "Build installable Linux systems from resources for bootable media and development targets",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Load config file if specified
			configFile, _ := cmd.Flags().GetString("config")
			if configFile != "" {
				loaded, err := config.Load(configFile)
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}
				cfg = *loaded
			}

			// Initialize logger
			logLevel, _ := cmd.Flags().GetString("log-level")
			level, err := logging.ParseLevel(logLevel)
			if err != nil {
				return fmt.Errorf("invalid log level: %w", err)
			}

			if cfg.Logging.File != "" {
				logger, err = logging.NewFromFile(level, cfg.Logging.File)
				if err != nil {
					return fmt.Errorf("failed to create logger: %w", err)
				}
			} else {
				logger = logging.New(level, os.Stderr)
			}

			return nil
		},
	}

	var labels []string
	var memory uint
	var cpus uint
	var org string
	var repo string
	var appID int64
	var privateKeyPath string
	var group string
	var token string
	var configFile string
	var logLevel string
	var username string

	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Config file path")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new runner VM",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			var ghApp *github.App
			if appID > 0 && privateKeyPath != "" {
				keyData, err := os.ReadFile(privateKeyPath)
				if err != nil {
					return fmt.Errorf("failed to read private key: %w", err)
				}
				ghApp, err = github.NewApp(appID, keyData, org)
				if err != nil {
					return fmt.Errorf("failed to create GitHub App: %w", err)
				}
			}
			var tokenProvider github.TokenProvider
			if ghApp != nil {
				tokenProvider = ghApp
			} else {
				tokenProvider = github.NewCLI(org)
			}
			if token == "" {
				registration, err := tokenProvider.GetRunnerRegistrationToken()
				if err != nil {
					return err
				}
				token = registration.Token
			}
			mgr := runner.NewManager(client)

			callbackPath, err := phoneHomeBinaryPath()
			if err != nil {
				return err
			}
			if err := mgr.EnsureInfrastructure(callbackPath); err != nil {
				return err
			}

			cfg := runner.DefaultConfig()
			cfg.Organization = org
			cfg.Repository = repo
			cfg.Username = username
			cfg.Labels = labels
			cfg.MemoryMB = memory
			cfg.CPUs = cpus
			cfg.Group = group

			if err := mgr.CreateWithCloudInit(cfg, token); err != nil {
				return err
			}

			if err := mgr.Start(cfg.Name); err != nil {
				return err
			}

			logger.Info("Created: %s, waiting for phone-home...", cfg.Name)
			ph, err := mgr.WaitForPhoneHome(cfg.Name, 15*time.Minute)
			if err != nil {
				return fmt.Errorf("VM started but never phoned home: %w", err)
			}
			logger.Info("Ready: %s (vsock CID %d)", cfg.Name, ph.CID)

			return nil
		},
	}

	createCmd.Flags().StringSliceVarP(&labels, "labels", "l", nil, "Runner labels")
	createCmd.Flags().UintVarP(&memory, "memory", "m", 4096, "Memory in MB")
	createCmd.Flags().UintVarP(&cpus, "cpus", "c", 2, "Number of CPUs")
	createCmd.Flags().StringVarP(&org, "org", "o", os.Getenv("GH_RUNNER_ORG"), "GitHub organization")
	createCmd.Flags().StringVarP(&repo, "repo", "r", "", "GitHub repository")
	createCmd.Flags().Int64Var(&appID, "app-id", 0, "GitHub App ID")
	createCmd.Flags().StringVar(&privateKeyPath, "private-key", "", "Path to GitHub App private key")
	createCmd.Flags().StringVarP(&group, "group", "g", "", "Runner group")
	createCmd.Flags().StringVar(&token, "token", "", "GitHub runner registration token")
	createCmd.Flags().StringVarP(&username, "username", "u", os.Getenv("GH_USERNAME"), "GitHub username for SSH key import")

	startCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Start a runner VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			mgr := runner.NewManager(client)
			logger.Info("Starting runner %s", args[0])
			if err := mgr.Start(args[0]); err != nil {
				return err
			}

			logger.Info("Started: %s", args[0])
			return nil
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop [name]",
		Short: "Stop a runner VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			mgr := runner.NewManager(client)
			logger.Info("Stopping runner %s", args[0])
			if err := mgr.Stop(args[0]); err != nil {
				return err
			}

			logger.Info("Stopped: %s", args[0])
			return nil
		},
	}

	destroyCmd := &cobra.Command{
		Use:   "destroy [name]",
		Short: "Destroy a runner VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			mgr := runner.NewManager(client)
			logger.Info("Destroying runner %s", args[0])
			if err := mgr.Destroy(args[0]); err != nil {
				return err
			}

			logger.Info("Destroyed: %s", args[0])
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all runner VMs",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			mgr := runner.NewManager(client)
			domains, err := mgr.List()
			if err != nil {
				return err
			}

			if len(domains) == 0 {
				fmt.Println("No runner VMs found")
				return nil
			}

			fmt.Printf("%-20s %-10s %-10s %-10s\n", "NAME", "STATE", "MEMORY", "CPUS")
			fmt.Println("------------------------------------------------------------")
			for _, d := range domains {
				fmt.Printf("%-20s %-10s %-10d %-10d\n", d.Name, d.State, d.MemoryMB, d.CPUs)
			}

			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status [name]",
		Short: "Show runner VM status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			mgr := runner.NewManager(client)
			status, err := mgr.Status(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Name: %s\n", status.Name)
			fmt.Printf("State: %s\n", status.State)
			fmt.Printf("UUID: %s\n", status.UUID)
			fmt.Printf("Memory: %d MB\n", status.MemoryMB)
			fmt.Printf("CPUs: %d\n", status.CPUs)
			if status.IP != "" {
				fmt.Printf("IP: %s\n", status.IP)
			}
			disk, err := mgr.DiskInfo(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Disk capacity: %d MiB\n", disk.Capacity/(1024*1024))
			fmt.Printf("Disk allocation: %d MiB\n", disk.Allocation/(1024*1024))

			return nil
		},
	}

	waitCmd := &cobra.Command{
		Use:   "wait [name]",
		Short: "Wait for a runner VM to be ready",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			mgr := runner.NewManager(client)
			logger.Info("Waiting for runner %s to be ready", args[0])
			ip, err := mgr.WaitForReady(args[0], 2*time.Minute)
			if err != nil {
				return err
			}

			logger.Info("Ready: %s (%s)", args[0], ip)
			return nil
		},
	}

	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check runner health",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			checker := health.NewChecker(client, 30*time.Second, 2*time.Minute)
			statuses, err := checker.HealthCheck(cmd.Context())
			if err != nil {
				return err
			}

			if len(statuses) == 0 {
				fmt.Println("No runners found")
				return nil
			}

			fmt.Printf("%-20s %-10s %-10s %-15s\n", "NAME", "HEALTHY", "STATE", "IP")
			fmt.Println("------------------------------------------------------------")
			for _, s := range statuses {
				healthy := "no"
				if s.Healthy {
					healthy = "yes"
				}
				fmt.Printf("%-20s %-10s %-10s %-15s\n", s.Name, healthy, s.State, s.IP)
			}

			return nil
		},
	}

	consoleCmd := &cobra.Command{
		Use:   "console [name]",
		Short: "Open a serial console to a runner VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()
			return runner.NewManager(client).Console(args[0], os.Stdout)
		},
	}

	dumpxmlCmd := &cobra.Command{
		Use:   "dumpxml [name]",
		Short: "Dump domain XML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()

			xml, err := client.GetDomainXML(args[0])
			if err != nil {
				return err
			}
			fmt.Println(xml)
			return nil
		},
	}

	imagesCmd := &cobra.Command{
		Use:   "images",
		Short: "Manage cloud images",
	}

	imagesListCmd := &cobra.Command{
		Use:   "list",
		Short: "List available cloud images",
		RunE: func(cmd *cobra.Command, args []string) error {
			distro, _ := cmd.Flags().GetString("distro")
			release, _ := cmd.Flags().GetString("release")
			lts, _ := cmd.Flags().GetBool("lts")
			arch, _ := cmd.Flags().GetString("arch")

			filter := images.Filter{
				Distro:  distro,
				Release: release,
				LTS:     lts,
				Arch:    arch,
			}

			imgs, err := images.ListImages(filter)
			if err != nil {
				return err
			}

			if len(imgs) == 0 {
				fmt.Println("No images found matching filter")
				return nil
			}

			fmt.Printf("%-8s %-9s %-18s %-10s %-11s %-10s %-10s %-13s %-8s %s\n", "DISTRO", "RELEASE", "CODENAME", "ARCH", "SUPPORT", "RELEASED", "BUILT", "SIZE", "FORMAT", "IMAGE")
			for _, img := range imgs {
				fmt.Printf("%-8s %-9s %-18s %-10s %-11s %-10s %-10s %-13s %-8s %s\n", img.Distro, img.Release, img.Codename, img.Arch, img.Support, imageDate(img.ReleaseDate), imageDate(img.BuildDate), imageSize(img.Size), img.Format, img.Name)
			}

			return nil
		},
	}

	imagesListCmd.Flags().StringP("distro", "d", "ubuntu", "Distribution (ubuntu, debian, fedora, all)")
	imagesListCmd.Flags().StringP("release", "r", "", "Release version (e.g., 26.04)")
	imagesListCmd.Flags().BoolP("lts", "l", false, "Only show LTS releases")
	imagesListCmd.Flags().StringP("arch", "a", "amd64", "Architecture (amd64, arm64)")

	imagesSelectCmd := &cobra.Command{
		Use:   "select [distro]",
		Short: "Select one supported cloud image",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			distro := ""
			if len(args) == 1 {
				distro = args[0]
			}
			release, _ := cmd.Flags().GetString("release")
			arch, _ := cmd.Flags().GetString("arch")
			format, _ := cmd.Flags().GetString("format")
			lts, _ := cmd.Flags().GetBool("lts")
			image, err := images.SelectImage(images.Selector{
				Distro: distro, Release: release, Arch: arch, Format: format, LTS: lts,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Distro: %s\nRelease: %s\nCodename: %s\nArchitecture: %s\nSupport: %s\nReleased: %s\nBuilt: %s\nFormat: %s\nSize: %s\nImage: %s\nURL: %s\nChecksum: %s:%s\n",
				image.Distro, image.Release, image.Codename, image.Arch, image.Support,
				imageDate(image.ReleaseDate), imageDate(image.BuildDate), image.Format,
				imageSize(image.Size), image.Name, image.URL, image.ChecksumType, image.Checksum)
			return nil
		},
	}
	imagesSelectCmd.Flags().StringP("release", "r", "", "Release version or codename")
	imagesSelectCmd.Flags().StringP("arch", "a", "", "Architecture (defaults to host architecture)")
	imagesSelectCmd.Flags().StringP("format", "f", "", "Image format")
	imagesSelectCmd.Flags().BoolP("lts", "l", false, "Require an LTS release")

	imagesCmd.AddCommand(imagesListCmd, imagesSelectCmd)

	rootCmd.AddCommand(createCmd, startCmd, stopCmd, destroyCmd, listCmd, statusCmd, waitCmd, healthCmd, consoleCmd, dumpxmlCmd, imagesCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func phoneHomeBinaryPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate shuttle executable: %w", err)
	}
	path := filepath.Join(filepath.Dir(executable), "distaff")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("phone-home binary %q is unavailable: %w", path, err)
	}
	return path, nil
}
