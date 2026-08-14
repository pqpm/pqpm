(() => {
  const $ = (sel) => document.querySelector(sel);
  const flashEl = $("#flash");
  const svcBody = $("#svc-body");
  const svcNames = $("#svc-names");
  const daemonStatus = $("#daemon-status");
  const userLabel = $("#user-label");
  const logView = $("#log-view");
  const logName = $("#log-name");
  const configEditor = $("#config-editor");
  const configPath = $("#config-path");
  const confirmDlg = $("#confirm-dlg");
  const confirmMsg = $("#confirm-msg");

  function flash(msg, ok = true) {
    flashEl.hidden = false;
    flashEl.textContent = msg;
    flashEl.className = "flash " + (ok ? "ok" : "err");
  }

  async function api(path, opts = {}) {
    const res = await fetch(path, {
      headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
      ...opts,
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || data.ok === false) {
      const err = new Error(data.error || data.message || res.statusText);
      err.data = data;
      throw err;
    }
    return data;
  }

  function confirmAction(message) {
    return new Promise((resolve) => {
      confirmMsg.textContent = message;
      confirmDlg.addEventListener(
        "close",
        () => resolve(confirmDlg.returnValue === "ok"),
        { once: true }
      );
      confirmDlg.showModal();
    });
  }

  function statusClass(status) {
    const s = (status || "").toLowerCase();
    if (s === "running") return "running";
    if (s === "crashed") return "crashed";
    return "stopped";
  }

  function renderServices(services) {
    svcNames.innerHTML = "";
    if (!services || services.length === 0) {
      svcBody.innerHTML = `<tr><td colspan="7" class="muted">No services running. Define them in ~/.pqpm.toml and start one.</td></tr>`;
      return;
    }
    svcBody.innerHTML = services
      .map((s) => {
        const opt = document.createElement("option");
        opt.value = s.name;
        svcNames.appendChild(opt);
        return `<tr>
          <td><strong>${escapeHtml(s.name)}</strong><div class="muted mono">${escapeHtml(s.command || "")}</div></td>
          <td><span class="status-dot ${statusClass(s.status)}"></span> ${escapeHtml(s.status || "")}</td>
          <td class="mono">${s.pid || "—"}</td>
          <td class="mono">${escapeHtml(s.memory_usage || "—")}</td>
          <td class="mono">${escapeHtml(s.cpu_usage || "—")}</td>
          <td class="mono">${s.restarts ?? 0}</td>
          <td>
            <div class="actions">
              <button type="button" data-act="start" data-name="${escapeAttr(s.name)}">Start</button>
              <button type="button" data-act="stop" data-name="${escapeAttr(s.name)}" class="danger">Stop</button>
              <button type="button" data-act="restart" data-name="${escapeAttr(s.name)}">Restart</button>
              <button type="button" data-act="log" data-name="${escapeAttr(s.name)}" class="ghost">Log</button>
            </div>
          </td>
        </tr>`;
      })
      .join("");
  }

  function escapeHtml(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function escapeAttr(s) {
    return escapeHtml(s).replace(/'/g, "&#39;");
  }

  async function refresh() {
    try {
      const me = await api("/api/me");
      userLabel.textContent = me.username ? `as ${me.username}` : "";
    } catch (_) {
      /* ignore */
    }

    try {
      await api("/api/ping");
      daemonStatus.textContent = "daemon online";
      daemonStatus.className = "pill ok";
    } catch (e) {
      daemonStatus.textContent = "daemon unreachable";
      daemonStatus.className = "pill bad";
      flash(e.message || "Daemon unreachable", false);
    }

    try {
      const data = await api("/api/status");
      renderServices(data.services || []);
    } catch (e) {
      svcBody.innerHTML = `<tr><td colspan="7" class="muted">${escapeHtml(e.message)}</td></tr>`;
    }

    try {
      const cfg = await api("/api/config");
      configEditor.value = cfg.content || "";
      if (cfg.path) {
        configPath.innerHTML = `Edit <code>${escapeHtml(cfg.path)}</code>. Dangerous shell operators are rejected.`;
      }
    } catch (e) {
      flash(e.message || "Could not load config", false);
    }
  }

  async function doAction(act, name) {
    if (act === "log") {
      logName.value = name;
      await loadLog();
      return;
    }
    const label = act.charAt(0).toUpperCase() + act.slice(1);
    if (!(await confirmAction(`${label} service “${name}”?`))) return;
    try {
      const data = await api(`/api/${act}`, {
        method: "POST",
        body: JSON.stringify({ name }),
      });
      flash(data.message || `${label} OK`);
      await refresh();
    } catch (e) {
      flash(e.message || `${label} failed`, false);
    }
  }

  async function loadLog() {
    const name = logName.value.trim();
    if (!name) {
      flash("Enter a service name for logs", false);
      return;
    }
    try {
      const data = await api(`/api/log?name=${encodeURIComponent(name)}&lines=200`);
      logView.textContent = data.log || "(empty log)";
    } catch (e) {
      logView.textContent = e.message || "Failed to load log";
      flash(e.message || "Log failed", false);
    }
  }

  svcBody.addEventListener("click", (ev) => {
    const btn = ev.target.closest("button[data-act]");
    if (!btn) return;
    doAction(btn.dataset.act, btn.dataset.name);
  });

  $("#btn-refresh").addEventListener("click", () => refresh());
  $("#btn-log").addEventListener("click", () => loadLog());
  $("#btn-save-config").addEventListener("click", async () => {
    try {
      const data = await api("/api/config", {
        method: "POST",
        body: JSON.stringify({
          content: configEditor.value,
          reload: $("#config-reload").checked,
        }),
      });
      flash(data.message || "Config saved");
      if (data.reload_error) flash(data.reload_error, false);
      await refresh();
    } catch (e) {
      flash(e.message || "Save failed", false);
    }
  });

  refresh();
  setInterval(refresh, 15000);
})();
