# PQPM: Simple & Secure Process Manager

PQPM (Process Queue Process Manager) is a lightweight, system-level daemon written in Go, designed for VPS environments like Virtualmin. It allows non-root users to manage their own long-running processes, binaries, and cron-like tasks without requiring sudo or root access.

Unlike PM2, which focuses on Node.js, **PQPM is language-agnostic**. Whether it's a Go binary, a PHP queue worker, or a Python script, PQPM handles the execution, monitoring, and automatic restarts under the specific user's permissions.

## Key Features

- **Zero-Root for Users** — Users manage their services via a CLI tool without ever needing root privileges.
- **Daemon-Level Security** — The parent daemon runs as root to spawn processes but drops privileges to the specific user (UID/GID) instantly.
- **Resource Throttling** — Built-in support for Linux cgroups to prevent runaway processes from consuming all RAM or CPU.
- **Auto-Restart** — Automatically restarts processes on crash or system reboot.
- **Simple Configuration** — Uses human-readable TOML files stored in the user's home directory.
- **Asynchronous Management** — Built with Go routines and channels for high-performance, non-blocking process monitoring.

## How It Works

1. **The Daemon** — A root-level service (`pqpmd`) runs in the background.

2. **The Config** — A user creates a `.pqpm.toml` file in their home directory:

   ```toml
   [service.my-worker]
   command = "/usr/bin/php /home/user/public_html/artisan queue:work"
   restart = "always"
   max_memory = "512MB"
   cpu_limit = "20%"
   ```

3. **The Interaction** — The user runs `pqpm start my-worker`.

4. **Verification** — The daemon identifies the user via Unix Domain Sockets (`SO_PEERCRED`), validates the path, and spawns the process safely.

## Installation

### Quick Install (Recommended)

Run the one-liner on your Linux server:

```bash
curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install.sh | sudo bash
```

This will automatically:
- Detect your architecture (amd64/arm64)
- Download the latest release
- Install `pqpmd` and `pqpm` to `/usr/local/bin`
- Create runtime directories
- Set up the systemd service

### Download from GitHub Releases

Grab a specific version manually:

```bash
# Download (replace v0.1.0 and linux-amd64 with your version/arch)
curl -LO https://github.com/pqpm/pqpm/releases/download/v0.1.0/pqpm-v0.1.0-linux-amd64.tar.gz

# Verify checksum
curl -LO https://github.com/pqpm/pqpm/releases/download/v0.1.0/checksums.txt
sha256sum -c checksums.txt

# Extract and install
tar xzf pqpm-v0.1.0-linux-amd64.tar.gz
sudo install -m 0755 pqpmd /usr/local/bin/
sudo install -m 0755 pqpm /usr/local/bin/
```

### Build from Source

Requires Go 1.21+:

```bash
git clone https://github.com/pqpm/pqpm.git
cd pqpm
make build          # Binaries output to ./bin/
sudo make install   # Install to /usr/local/bin + create runtime dirs
```

## Quick Start

**1. Start the daemon:**

```bash
sudo systemctl enable --now pqpmd
```

**2. Create your config file:**

```bash
cp /path/to/example.pqpm.toml ~/.pqpm.toml
nano ~/.pqpm.toml
```

Example `~/.pqpm.toml`:

```toml
[service.my-worker]
command = "/usr/bin/php /home/user/public_html/artisan queue:work"
restart = "always"
max_memory = "512MB"
cpu_limit = "20%"

[service.api-server]
command = "/home/user/bin/api-server --port 8080"
restart = "on-failure"
max_memory = "1GB"
cpu_limit = "50%"
working_dir = "/home/user/api"
```

**3. Start a service:**

```bash
pqpm start my-worker
```

**4. Check status:**

```bash
pqpm status
```

## Commands

| Command | Description |
|---|---|
| `pqpm status` | View all running processes for the current user |
| `pqpm start <name>` | Register and start a service from your config file |
| `pqpm stop <name>` | Stop a running service |
| `pqpm restart <name>` | Restart a specific service |
| `pqpm log <name>` | View output/error logs for a process |
| `pqpm version` | Print the installed version |

## Configuration Reference

Each service is defined as a `[service.<name>]` block in `~/.pqpm.toml`:

| Field | Required | Default | Description |
|---|---|---|---|
| `command` | ✅ | — | Full command to execute |
| `restart` | ❌ | `"always"` | Restart policy: `"always"`, `"on-failure"`, or `"never"` |
| `max_memory` | ❌ | — | Memory limit (e.g. `"512MB"`, `"1GB"`) |
| `cpu_limit` | ❌ | — | CPU limit as percentage (e.g. `"20%"`) |
| `working_dir` | ❌ | — | Working directory for the process |
| `log_file` | ❌ | — | Custom log file path |

## Security & Safety

PQPM is built with a **safety-first** mindset:

- **Identity Validation** — Uses kernel-level socket credentials (`SO_PEERCRED`) to ensure User A cannot stop User B's processes.
- **Privilege Separation** — The daemon runs as root only to spawn processes; it immediately drops to the target user's UID/GID.
- **Resource Limits** — Hard limits on memory and CPU via cgroups prevent runaway processes from crashing the VPS.
- **Path Restricted** — Processes are restricted to running within the user's authorized directories.

## System Requirements

- **OS:** Linux (kernel 3.5+ for cgroup v2 support)
- **Architecture:** amd64 or arm64
- **Privileges:** The daemon (`pqpmd`) must run as root; the CLI (`pqpm`) runs as a normal user

## Project Structure

```
pqpm/
├── cmd/
│   ├── cli/            # pqpm CLI binary
│   └── daemon/         # pqpmd daemon binary
├── internal/
│   ├── cgroup/         # Linux cgroup resource limits
│   ├── config/         # TOML config loading & validation
│   ├── daemon/         # Request handler & dispatcher
│   ├── logger/         # Structured logging (zap)
│   ├── process/        # Process lifecycle manager
│   ├── socket/         # Unix socket + peer credentials
│   ├── types/          # Shared type definitions
│   └── version/        # Build-time version info
├── init/
│   └── pqpmd.service   # Systemd unit file
├── install.sh          # One-liner install script
├── example.pqpm.toml   # Example user config
├── Makefile            # Build, test, install targets
└── README.md
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

```bash
# Development workflow
make fmt      # Format code
make vet      # Run go vet
make test     # Run tests with race detection
make build    # Build binaries
```

## License

MIT License. See [LICENSE](LICENSE) for details.