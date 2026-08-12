#!/usr/bin/env bash
set -euo pipefail

mkdir -p /var/run/pqpm /var/log/pqpm/users /var/lib/pqpm /home/foliyo/api-bundle/logs

# Minimal app that stays foreground and writes to stdout (captured by pqpm log_file).
cat > /home/foliyo/api-bundle/foliyo-cloud << 'APP'
#!/bin/bash
echo "Foliyo Cloud API on http://0.0.0.0:9100"
echo "  Env:    working"
i=0
while true; do
  i=$((i + 1))
  echo "heartbeat $i"
  sleep 1
done
APP
chmod +x /home/foliyo/api-bundle/foliyo-cloud

# One-shot helper for restart=never tests
cat > /home/foliyo/api-bundle/oneshot << 'APP'
#!/bin/bash
echo "oneshot ran"
exit 0
APP
chmod +x /home/foliyo/api-bundle/oneshot

# Crash-loop helper (exits immediately) for stop-race coverage
cat > /home/foliyo/api-bundle/crashy << 'APP'
#!/bin/bash
echo "crashy starting"
exit 1
APP
chmod +x /home/foliyo/api-bundle/crashy

cat > /home/foliyo/.pqpm.toml << 'TOML'
[service.foliyo]
command = "./foliyo-cloud"
restart = "always"
max_memory = "256MB"
cpu_limit = "40%"
working_dir = "/home/foliyo/api-bundle"
log_file = "./logs/foliyo.log"
env = { FOLIYO_PORT = "9100", NODE_ENV = "production" }

[service.oneshot]
command = "./oneshot"
restart = "never"
working_dir = "/home/foliyo/api-bundle"
log_file = "./logs/oneshot.log"

[service.crashy]
command = "./crashy"
restart = "always"
working_dir = "/home/foliyo/api-bundle"
log_file = "./logs/crashy.log"
TOML

chown -R foliyo:foliyo /home/foliyo

# Start daemon in background
pqpmd &
DAEMON_PID=$!

# Wait for socket
for i in $(seq 1 50); do
  if [[ -S /var/run/pqpm/pqpmd.sock ]]; then
    break
  fi
  sleep 0.1
done

if [[ ! -S /var/run/pqpm/pqpmd.sock ]]; then
  echo "FAIL: pqpmd socket not created"
  exit 1
fi

# Give socket world/group access for non-root CLI (daemon should set this; ensure usable)
chmod 666 /var/run/pqpm/pqpmd.sock 2>/dev/null || true

exec "$@"
