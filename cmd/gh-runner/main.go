package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/sovereignite/gh-workers/internal/libvirt"
	"github.com/sovereignite/gh-workers/internal/runner"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gh-runner",
		Short: "GitHub Actions runner VM manager",
		Long:  "Manage self-hosted GitHub Actions runner VMs using libvirt",
	}

	var count int
	var labels []string
	var memory uint
	var cpus uint
	var org string
	var repo string

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

			mgr := runner.NewManager(client)

			// Ensure infrastructure
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

				if err := mgr.Create(cfg); err != nil {
					return fmt.Errorf("failed to create %s: %w", vmName, err)
				}

				if err := mgr.Start(vmName); err != nil {
					return fmt.Errorf("failed to start %s: %w", vmName, err)
				}

				fmt.Printf("Created and started: %s\n", vmName)
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
			if err := mgr.Start(args[0]); err != nil {
				return err
			}

			fmt.Printf("Started: %s\n", args[0])
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
			if err := mgr.Stop(args[0]); err != nil {
				return err
			}

			fmt.Printf("Stopped: %s\n", args[0])
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
			if err := mgr.Destroy(args[0]); err != nil {
				return err
			}

			fmt.Printf("Destroyed: %s\n", args[0])
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
			ip, err := mgr.WaitForReady(args[0], 2*time.Minute)
			if err != nil {
				return err
			}

			fmt.Printf("Ready: %s (%s)\n", args[0], ip)
			return nil
		},
	}

	rootCmd.AddCommand(createCmd, startCmd, stopCmd, destroyCmd, listCmd, statusCmd, waitCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
