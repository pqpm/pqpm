#!/usr/bin/env bash
set -euo pipefail

PASS=0
FAIL=0

assert() {
  local desc="$1"
  shift
  if "$@"; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc"
    FAIL=$((FAIL + 1))
  fi
}

assert_contains() {
  local desc="$1"
  local haystack="$2"
  local needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc"
    echo "  expected to contain: $needle"
    echo "  got: $haystack"
    FAIL=$((FAIL + 1))
  fi
}

assert_not_contains() {
  local desc="$1"
  local haystack="$2"
  local needle="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc"
    echo "  expected NOT to contain: $needle"
    echo "  got: $haystack"
    FAIL=$((FAIL + 1))
  fi
}

run_as_user() {
  gosu foliyo "$@"
}

echo "=== PQPM e2e tests ==="

# --- ping ---
out=$(run_as_user pqpm ping)
assert_contains "daemon ping" "$out" "pong"

# --- start foliyo ---
out=$(run_as_user pqpm start foliyo)
assert_contains "start foliyo" "$out" "started successfully"

sleep 1.5

# --- status shows running ---
out=$(run_as_user pqpm status)
assert_contains "status lists foliyo" "$out" "foliyo"
assert_contains "status shows running" "$out" "running"

out=$(run_as_user pqpm status foliyo)
assert_contains "status foliyo filter" "$out" "foliyo"
assert_contains "filtered status running" "$out" "running"
assert_not_contains "memory not hardcoded zero-only when running" "$out" "running    0B"

# Memory line may still be small; just ensure process is alive and RSS path works.
# Accept either non-zero memory OR at least running with a live PID.
pid=$(echo "$out" | awk '/foliyo/ {print $2; exit}')
assert "status reports live PID" kill -0 "$pid"

# --- custom log_file + log command ---
assert "custom log file exists" test -f /home/foliyo/api-bundle/logs/foliyo.log
assert_contains "custom log has app output" "$(cat /home/foliyo/api-bundle/logs/foliyo.log)" "Foliyo Cloud API"

out=$(run_as_user pqpm log -n 20 foliyo 2>&1)
assert_contains "log shows path on stderr/stdout" "$out" "/home/foliyo/api-bundle/logs/foliyo.log"
assert_contains "log prints app output" "$out" "Foliyo Cloud API"
assert_contains "log shows heartbeats" "$out" "heartbeat"

# follow briefly then kill
timeout 2s gosu foliyo pqpm log -f -n 1 foliyo >/tmp/pqpm-follow.out 2>/tmp/pqpm-follow.err || true
assert_contains "follow saw path" "$(cat /tmp/pqpm-follow.err)" "/home/foliyo/api-bundle/logs/foliyo.log"
assert "follow produced output" test -s /tmp/pqpm-follow.out

# Default path should not be the only place (may or may not exist); custom must have content.
if [[ -f /var/log/pqpm/users/1042/foliyo.log ]]; then
  custom_lines=$(wc -l < /home/foliyo/api-bundle/logs/foliyo.log)
  assert "custom log received lines" test "$custom_lines" -gt 0
fi

# --- stop sticks (no restart storm) ---
out=$(run_as_user pqpm stop foliyo)
assert_contains "stop succeeds" "$out" "stopped"
sleep 3

# After stop, service should be gone from status (removed from map) or not running.
out=$(run_as_user pqpm status 2>&1 || true)
if echo "$out" | grep -q foliyo; then
  assert_not_contains "stopped service not running" "$out" "running"
  # If still listed, must not keep climbing restarts after stop window
  restarts=$(echo "$out" | awk '/foliyo/ {print $6; exit}')
  sleep 3
  out2=$(run_as_user pqpm status 2>&1 || true)
  restarts2=$(echo "$out2" | awk '/foliyo/ {print $6; exit}')
  assert "no restart after stop" test "${restarts:-0}" -eq "${restarts2:-0}"
else
  assert "stopped service removed from status" true
fi

# Process group should be dead
assert "foliyo process gone after stop" bash -c '! pgrep -u foliyo -f foliyo-cloud >/dev/null'

# --- restart after stop ---
out=$(run_as_user pqpm start foliyo)
assert_contains "start after stop" "$out" "started successfully"
sleep 1
out=$(run_as_user pqpm status foliyo)
assert_contains "running after restart start" "$out" "running"

out=$(run_as_user pqpm restart foliyo)
assert_contains "restart command" "$out" "restarted"
sleep 1
out=$(run_as_user pqpm status foliyo)
assert_contains "running after restart" "$out" "running"

# --- reload re-reads ~/.pqpm.toml ---
if ! grep -q RELOAD_MARKER /home/foliyo/.pqpm.toml; then
  sed -i 's/NODE_ENV = "production"/NODE_ENV = "production", RELOAD_MARKER = "yes"/' /home/foliyo/.pqpm.toml
fi
chown foliyo:foliyo /home/foliyo/.pqpm.toml
old_pid=$(run_as_user pqpm status foliyo | awk '/foliyo/ {print $2; exit}')
if out=$(run_as_user pqpm reload 2>&1); then
  echo "FAIL: bare reload should require name or --all"
  echo "  got: $out"
  FAIL=$((FAIL + 1))
else
  echo "PASS: bare reload rejected"
  PASS=$((PASS + 1))
fi

out=$(run_as_user pqpm reload foliyo)
assert_contains "reload single service" "$out" "Reloaded foliyo"
sleep 1
out=$(run_as_user pqpm status foliyo)
assert_contains "running after reload" "$out" "running"
new_pid=$(echo "$out" | awk '/foliyo/ {print $2; exit}')
assert "reload replaced process" test "$old_pid" != "$new_pid"

out=$(run_as_user pqpm reload --all)
assert_contains "reload all" "$out" "Reloaded"

# --- stop race with crash loop ---
out=$(run_as_user pqpm start crashy)
assert_contains "start crashy" "$out" "started successfully"
sleep 2
out=$(run_as_user pqpm status crashy 2>&1 || true)
# May be restarting or running briefly
assert_contains "crashy visible" "$out" "crashy"

out=$(run_as_user pqpm stop crashy)
assert_contains "stop crashy during restart loop" "$out" "stopped"
sleep 3
out=$(run_as_user pqpm status 2>&1 || true)
if echo "$out" | grep -q crashy; then
  assert_not_contains "crashy not running after stop" "$out" "running"
  r1=$(echo "$out" | awk '/crashy/ {print $6; exit}')
  sleep 3
  out2=$(run_as_user pqpm status 2>&1 || true)
  r2=$(echo "$out2" | awk '/crashy/ {print $6; exit}')
  assert "crashy restarts frozen after stop" test "${r1:-0}" -eq "${r2:-0}"
else
  assert "crashy removed after stop" true
fi

# --- oneshot never restart ---
out=$(run_as_user pqpm start oneshot)
assert_contains "start oneshot" "$out" "started successfully"
sleep 1
assert "oneshot log written" test -f /home/foliyo/api-bundle/logs/oneshot.log
assert_contains "oneshot log content" "$(cat /home/foliyo/api-bundle/logs/oneshot.log)" "oneshot ran"

# --- unknown service filter ---
if run_as_user pqpm status nosuch 2>/dev/null; then
  echo "FAIL: status nosuch should error"
  FAIL=$((FAIL + 1))
else
  echo "PASS: status unknown service errors"
  PASS=$((PASS + 1))
fi

# --- stop forgets (no separate untrack); daemon restart restores only tracked ---
restart_daemon() {
  if pidof pqpmd >/dev/null 2>&1; then
    kill -TERM $(pidof pqpmd) 2>/dev/null || true
    for _ in $(seq 1 50); do
      pidof pqpmd >/dev/null 2>&1 || break
      sleep 0.1
    done
    kill -KILL $(pidof pqpmd) 2>/dev/null || true
  fi
  rm -f /var/run/pqpm/pqpmd.sock
  pqpmd &
  for _ in $(seq 1 50); do
    [[ -S /var/run/pqpm/pqpmd.sock ]] && break
    sleep 0.1
  done
  chmod 666 /var/run/pqpm/pqpmd.sock 2>/dev/null || true
  # Wait until daemon accepts requests
  for _ in $(seq 1 50); do
    if gosu foliyo pqpm ping >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

state_has_service() {
  local name="$1"
  [[ -f /var/lib/pqpm/state.json ]] || return 1
  grep -q "\"name\": \"$name\"" /var/lib/pqpm/state.json
}

# Ensure foliyo is running and tracked
out=$(run_as_user pqpm status foliyo 2>&1 || true)
if ! echo "$out" | grep -q running; then
  run_as_user pqpm start foliyo >/dev/null
  sleep 1
fi
assert "tracked service present in state.json" state_has_service foliyo

# Daemon restart should restore still-tracked services
restart_daemon
assert "daemon came back after restart" gosu foliyo pqpm ping
sleep 1
out=$(run_as_user pqpm status foliyo 2>&1 || true)
assert_contains "tracked service restored after daemon restart" "$out" "running"

# Stop forgets: removed from state, not restored after daemon restart
out=$(run_as_user pqpm stop foliyo)
assert_contains "stop before forget check" "$out" "stopped"
sleep 1
assert "stopped service absent from state.json" bash -c '! grep -q "\"name\": \"foliyo\"" /var/lib/pqpm/state.json 2>/dev/null'

restart_daemon
assert "daemon came back after stop+restart" gosu foliyo pqpm ping
sleep 1
out=$(run_as_user pqpm status 2>&1 || true)
assert_not_contains "stopped service not restored after daemon restart" "$out" "foliyo"

# toml recipe remains; start works again without any untrack command
out=$(run_as_user pqpm start foliyo)
assert_contains "start after forget still works from toml" "$out" "started successfully"
sleep 1
out=$(run_as_user pqpm status foliyo)
assert_contains "running after re-start from toml" "$out" "running"
assert "re-tracked in state.json" state_has_service foliyo

echo
echo "=== Results: $PASS passed, $FAIL failed ==="
if [[ "$FAIL" -ne 0 ]]; then
  echo "--- debug ---"
  run_as_user pqpm status || true
  ps aux | grep -E 'foliyo|crashy|pqpmd' || true
  ls -la /home/foliyo/api-bundle/logs/ || true
  cat /var/lib/pqpm/state.json 2>/dev/null || true
  exit 1
fi
exit 0
