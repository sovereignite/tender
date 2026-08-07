package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/sovereignite/gh-workers/internal/config"
	"github.com/sovereignite/gh-workers/internal/github"
	"github.com/sovereignite/gh-workers/internal/health"
	"github.com/sovereignite/gh-workers/internal/libvirt"
	"github.com/sovereignite/gh-workers/internal/logging"
	"github.com/sovereignite/gh-workers/internal/runner"
)

func main() {
	cfg := config.DefaultConfig()
	var logger *logging.Logger

	rootCmd := &cobra.Command{
		Use:   "gh-runner",
		Short: "GitHub Actions runner VM manager",
		Long:  "Manage self-hosted GitHub Actions runner VMs using libvirt",
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

	var count int
	var labels []string
	var memory uint
	var cpus uint
	var org string
	var repo string
	var appID int64
	var privateKeyPath string
	var group string
	var cloudInit bool
	var token string
	var configFile string
	var logLevel string

	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Config file path")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")

	createCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new runner VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer client.Close()

			// Initialize GitHub App if credentials provided
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

			var mgr *runner.Manager
			if ghApp != nil {
				mgr = runner.NewManagerWithGitHub(client, ghApp)
			} else {
				mgr = runner.NewManager(client)
			}

			// Ensure infrastructure
			logger.Info("Ensuring infrastructure is ready")
			if err := mgr.EnsureInfrastructure(); err != nil {
				return err
			}

			// Create VMs
			for i := 0; i < count; i++ {
				vmName := name
				if count > 1 {
					vmName = fmt.Sprintf("%s-%d", name, i+1)
				}

				cfg := runner.DefaultConfig(vmName)
				cfg.Organization = org
				cfg.Repository = repo
				cfg.Labels = labels
				cfg.MemoryMB = memory
				cfg.CPUs = cpus
				cfg.Group = group

				logger.Info("Creating runner %s", vmName)
				if cloudInit {
					if err := mgr.CreateWithCloudInit(cfg, token); err != nil {
						return fmt.Errorf("failed to create %s: %w", vmName, err)
					}
				} else {
					if err := mgr.Create(cfg); err != nil {
						return fmt.Errorf("failed to create %s: %w", vmName, err)
					}
				}

				logger.Info("Starting runner %s", vmName)
				if err := mgr.Start(vmName); err != nil {
					return fmt.Errorf("failed to start %s: %w", vmName, err)
				}

				logger.Info("Created and started: %s", vmName)
			}

			return nil
		},
	}

	createCmd.Flags().IntVarP(&count, "count", "n", 1, "Number of VMs to create")
	createCmd.Flags().StringSliceVarP(&labels, "labels", "l", []string{"self-hosted", "linux", "x64"}, "Runner labels")
	createCmd.Flags().UintVarP(&memory, "memory", "m", 4096, "Memory in MB")
	createCmd.Flags().UintVarP(&cpus, "cpus", "c", 2, "Number of CPUs")
	createCmd.Flags().StringVarP(&org, "org", "o", "", "GitHub organization")
	createCmd.Flags().StringVarP(&repo, "repo", "r", "", "GitHub repository")
	createCmd.Flags().Int64Var(&appID, "app-id", 0, "GitHub App ID")
	createCmd.Flags().StringVar(&privateKeyPath, "private-key", "", "Path to GitHub App private key")
	createCmd.Flags().StringVarP(&group, "group", "g", "default", "Runner group")
	createCmd.Flags().BoolVar(&cloudInit, "cloud-init", false, "Use cloud-init for runner installation")
	createCmd.Flags().StringVar(&token, "token", "", "GitHub runner registration token")

	startCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Start a runner VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := libvirt.NewClient()
			if err != nil {
				return err
			}
			defer client.Close()

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
			defer client.Close()

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
			defer client.Close()

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
			defer client.Close()

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
			defer client.Close()

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
			defer client.Close()

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
			defer client.Close()

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

	rootCmd.AddCommand(createCmd, startCmd, stopCmd, destroyCmd, listCmd, statusCmd, waitCmd, healthCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
