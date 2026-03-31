package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pqpm/pqpm/internal/socket"
	"github.com/pqpm/pqpm/internal/types"
	"github.com/pqpm/pqpm/internal/version"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "pqpm",
		Short: "PQPM - Simple & Secure Process Manager",
		Long:  "PQPM (Process Queue Process Manager) is a lightweight process manager\nfor VPS environments. Manage long-running processes without root access.",
	}

	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(stopCmd())
	rootCmd.AddCommand(restartCmd())
	rootCmd.AddCommand(logCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "View all running processes for the current user",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := socket.SendRequest(&types.DaemonRequest{
				Action: "status",
			})
			if err != nil {
				return err
			}

			if !resp.Success {
				return fmt.Errorf("error: %s", resp.Message)
			}

			if len(resp.Services) == 0 {
				fmt.Println("No services running.")
				return nil
			}

			fmt.Printf("%-20s %-8s %-10s %-8s %s\n", "NAME", "PID", "STATUS", "RESTARTS", "COMMAND")
			fmt.Println("----------------------------------------------------------------------")
			for _, svc := range resp.Services {
				fmt.Printf("%-20s %-8d %-10s %-8d %s\n",
					svc.Name, svc.PID, svc.Status, svc.Restarts, svc.Command)
			}
			return nil
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Register and start a new service from config file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := socket.SendRequest(&types.DaemonRequest{
				Action:  "start",
				Service: args[0],
			})
			if err != nil {
				return err
			}

			if !resp.Success {
				return fmt.Errorf("error: %s", resp.Message)
			}

			fmt.Println(resp.Message)
			return nil
		},
	}
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a running service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := socket.SendRequest(&types.DaemonRequest{
				Action:  "stop",
				Service: args[0],
			})
			if err != nil {
				return err
			}

			if !resp.Success {
				return fmt.Errorf("error: %s", resp.Message)
			}

			fmt.Println(resp.Message)
			return nil
		},
	}
}

func restartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a specific service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := socket.SendRequest(&types.DaemonRequest{
				Action:  "restart",
				Service: args[0],
			})
			if err != nil {
				return err
			}

			if !resp.Success {
				return fmt.Errorf("error: %s", resp.Message)
			}

			fmt.Println(resp.Message)
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of pqpm",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.String())
		},
	}
}

func logCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "log <name>",
		Short: "View output/error logs for a process",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := socket.SendRequest(&types.DaemonRequest{
				Action:  "log",
				Service: args[0],
			})
			if err != nil {
				return err
			}

			if !resp.Success {
				return fmt.Errorf("error: %s", resp.Message)
			}

			fmt.Println(resp.Message)
			return nil
		},
	}
}
