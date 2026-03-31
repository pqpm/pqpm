PQPM: Simple & Secure Process Manager

PQPM (Process Queue Process Manager) is a lightweight, system-level daemon written in Go, designed for VPS environments like Virtualmin. It allows non-root users to manage their own long-running processes, binaries, and cron-like tasks without requiring sudo or root access.

Unlike PM2, which focuses on Node.js, PQPM is language-agnostic. Whether it's a Go binary, a PHP queue worker, or a Python script, PQPM handles the execution, monitoring, and automatic restarts under the specific user's permissions.
## Key Features

    Zero-Root for Users: Users manage their services via a CLI tool without ever needing root privileges.

    Daemon-Level Security: The parent daemon runs as root to spawn processes but drops privileges to the specific user (UID/GID) instantly.

    Resource Throttling: Built-in support for Linux cgroups to prevent runaway processes from consuming all RAM or CPU.

    Auto-Restart: Automatically restarts processes on crash or system reboot.

    Simple Configuration: Uses human-readable TOML/YAML files stored in the user's home directory.

    Asynchronous Management: Built with Go routines and channels for high-performance, non-blocking process monitoring.

## How It Works

    The Daemon: A root-level service (pqpmd) runs in the background.

    The Config: A user creates a .pqpm.toml file in their directory:
    Ini, TOML

    [service.my-worker]
    command = "/usr/bin/php /home/user/public_html/artisan queue:work"
    restart = "always"
    max_memory = "512MB"
    cpu_limit = "20%"

    The Interaction: The user runs pqpm start my-worker.

    Verification: The daemon identifies the user via Unix Domain Sockets (SO_PEERCRED), validates the path, and spawns the process safely.

## Installation (Coming Soon)
Bash

# Clone the repository
git clone https://github.com/pqpm/pqpm.git

# Build the daemon and CLI
go build -o pqpmd ./cmd/daemon
go build -o pqpm ./cmd/cli

## Commands

    pqpm status: View all running processes for the current user.

    pqpm start <name>: Register and start a new service from a config file.

    pqpm restart <name>: Restart a specific service.

    pqpm stop <name>: Kill a running service.

    pqpm log <name>: View output/error logs for a process.

## Security & Safety

PQPM is built with a Safety-First mindset:

    Identity Validation: Uses kernel-level socket credentials to ensure User A cannot stop User B's processes.

    Resource Limits: Hard limits on memory and CPU prevent "Infinite Loop" bugs from crashing the VPS.

    Path Restricted: Processes are restricted to running within the user's authorized directories.

## License

MIT License. Feel free to contribute!
