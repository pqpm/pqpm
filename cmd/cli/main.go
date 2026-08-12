package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
	rootCmd.AddCommand(reloadCmd())
	rootCmd.AddCommand(logCmd())
	rootCmd.AddCommand(pingCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [name]",
		Short: "View processes for the current user (optionally filter by service name)",
		Args:  cobra.MaximumNArgs(1),
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

			services := resp.Services
			if len(args) == 1 {
				name := args[0]
				filtered := services[:0]
				for _, svc := range services {
					if svc.Name == name {
						filtered = append(filtered, svc)
					}
				}
				services = filtered
				if len(services) == 0 {
					return fmt.Errorf("service %q not found", name)
				}
			}

			if len(services) == 0 {
				fmt.Println("No services running.")
				return nil
			}

			fmt.Printf("%-20s %-8s %-10s %-10s %-10s %-8s %s\n", "NAME", "PID", "STATUS", "MEMORY", "CPU", "RESTARTS", "COMMAND")
			fmt.Println("-------------------------------------------------------------------------------------------------")
			for _, svc := range services {
				fmt.Printf("%-20s %-8d %-10s %-10s %-10s %-8d %s\n",
					svc.Name, svc.PID, svc.Status, svc.MemoryUsage, svc.CPUUsage, svc.Restarts, svc.Command)
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

func reloadCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "reload <name>",
		Short: "Re-read ~/.pqpm.toml and restart a service with the new config",
		Long:  "Reloads the user's ~/.pqpm.toml and restarts the named service.\nUse --all to reload every managed service for this user.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all && len(args) != 1 {
				return fmt.Errorf("service name required (or pass --all)\n\nUsage:\n  pqpm reload <name>\n  pqpm reload --all")
			}
			if all && len(args) == 1 {
				return fmt.Errorf("specify either a service name or --all, not both")
			}

			req := &types.DaemonRequest{Action: "reload"}
			if all {
				req.Service = "*" // daemon treats "*" as reload-all
			} else {
				req.Service = args[0]
			}

			resp, err := socket.SendRequest(req)
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
	cmd.Flags().BoolVar(&all, "all", false, "Reload all managed services for this user")
	return cmd
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

func pingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Check if the daemon is responsive",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := socket.SendRequest(&types.DaemonRequest{
				Action: "ping",
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

func logCmd() *cobra.Command {
	var follow bool
	var lines int

	cmd := &cobra.Command{
		Use:   "log <name>",
		Short: "View or follow logs for a process",
		Long:  "Prints the last N lines of the service log (default 100).\nUse -f/--follow to stream new output like tail -f.",
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

			logPath := resp.LogPath
			if logPath == "" {
				// Backward compatible with older daemons that only set Message.
				logPath = strings.TrimPrefix(resp.Message, "Log file: ")
			}
			if logPath == "" {
				return fmt.Errorf("daemon did not return a log path")
			}

			if _, err := os.Stat(logPath); err != nil {
				return fmt.Errorf("cannot read log file %s: %w", logPath, err)
			}

			fmt.Fprintf(os.Stderr, "Log file: %s\n", logPath)

			if err := printLastLines(logPath, lines); err != nil {
				return err
			}
			if follow {
				return followFile(logPath)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "Number of lines to show from the end")
	return cmd
}

func printLastLines(path string, n int) error {
	if n <= 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	content := string(data)
	all := strings.Split(content, "\n")
	// Drop trailing empty element from final newline.
	if len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	for _, line := range all {
		fmt.Println(line)
	}
	return nil
}

func followFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	reader := bufio.NewReader(f)
	for {
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr)
			return nil
		default:
		}

		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			fmt.Print(line)
		}
		if err == io.EOF {
			time.Sleep(200 * time.Millisecond)
			fi, statErr := os.Stat(path)
			cur, seekErr := f.Seek(0, io.SeekCurrent)
			if statErr == nil && seekErr == nil && fi.Size() < cur {
				_ = f.Close()
				f, err = os.Open(path)
				if err != nil {
					return err
				}
				reader = bufio.NewReader(f)
			}
			continue
		}
		if err != nil {
			return err
		}
	}
}
