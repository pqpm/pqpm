# PQPM UI Addon

Optional **standalone** web UI for [PQPM](https://github.com/pqpm/pqpm). Works on any Linux VPS — **Webmin/Virtualmin is not required**.

Core `pqpmd` / `pqpm` stay unchanged. This addon talks to the daemon the same way the CLI does: **Unix socket + `SO_PEERCRED`**. Linux passwords (login mode) only create a UI session; they are never sent to the daemon.

## Install

Core PQPM must already be installed and `pqpmd` running.

```bash
# From a git checkout
sudo ./install-ui.sh --from-source

# Or from GitHub releases
curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash
```

Optional Webmin / Virtualmin module:

```bash
sudo ./install-ui.sh --from-source --with-webmin
# or from a release:
curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash -s -- --with-webmin
```

Then in the panel: **Webmin → Webmin Configuration → Refresh Modules**. The module appears under **Webmin → Servers → PQPM Process Manager** (or **Un-used Modules** until `pqpm` is on PATH).

## Uninstall (addon only)

Removes `pqpm-ui`, the systemd unit, and the Webmin module. Core `pqpmd` / `pqpm` stay installed.

```bash
# Install UI + Webmin/Virtualmin module
curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash -s -- --with-webmin

# Remove UI + Webmin module (core pqpm stays)
curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash -s -- --uninstall
```

## Run (any VPS)

```bash
# Multi-user: Linux account login, then manage that user's processes
sudo pqpm-ui -listen 127.0.0.1:9090 -auth login

# Single-user: no password, current UID only
pqpm-ui -listen 127.0.0.1:9090 -auth local
```

Open `http://127.0.0.1:9090`. Prefer localhost + SSH tunnel or a reverse proxy with TLS.

```bash
sudo systemctl enable --now pqpm-ui
```

### Features

- Status dashboard with start / stop / restart
- Log viewer
- `~/.pqpm.toml` editor (rejects dangerous shell operators)
- JSON API under `/api/*`

## Optional: Webmin / Virtualmin

Pass `--with-webmin` to `install-ui.sh`. The installer locates Webmin via `/etc/webmin/miniserv.conf` (including `/usr/libexec/webmin` on RHEL/Fedora Virtualmin) and registers the module with `install-module.pl`.

The module directory (e.g. `/usr/libexec/webmin/pqpm`) is only Webmin CGI. It calls the real CLI at `/usr/local/bin/pqpm` from `install.sh` — they do not need to live in the same folder.

## Layout

```
addons/ui/
  cmd/pqpm-ui/          # HTTP server + rpc helper
  internal/             # auth, client, server, web assets, rpc
  systemd/pqpm-ui.service
  webmin/               # optional panel module
```

This tree must not be imported by `cmd/daemon`, `cmd/cli`, or `internal/daemon`.
