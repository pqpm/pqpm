package daemon

import (
	"fmt"
	"net"
	"os/user"
	"strconv"
	"strings"

	"github.com/pqpm/pqpm/internal/config"
	"github.com/pqpm/pqpm/internal/logger"
	"github.com/pqpm/pqpm/internal/process"
	"github.com/pqpm/pqpm/internal/socket"
	"github.com/pqpm/pqpm/internal/types"
)

// Handler processes incoming client connections.
type Handler struct {
	Manager *process.Manager
}

// NewHandler creates a new request handler.
func NewHandler(mgr *process.Manager) *Handler {
	return &Handler{Manager: mgr}
}

// HandleConnection reads a request from the connection, authenticates the
// caller via peer credentials, and dispatches to the appropriate action.
func (h *Handler) HandleConnection(conn net.Conn) {
	defer conn.Close()

	cred, err := socket.GetPeerCred(conn)
	if err != nil {
		logger.Log.Warn("Failed to get peer credentials", "error", err)
		socket.WriteResponse(conn, &types.DaemonResponse{
			Success: false,
			Message: "Failed to verify identity: " + err.Error(),
		})
		return
	}

	logger.Log.Debug("Connection received",
		"uid", cred.UID,
		"gid", cred.GID,
		"pid", cred.PID,
	)

	req, err := socket.ReadRequest(conn)
	if err != nil {
		logger.Log.Warn("Failed to read request", "error", err)
		socket.WriteResponse(conn, &types.DaemonResponse{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	var resp *types.DaemonResponse
	switch req.Action {
	case "start":
		resp = h.handleStart(req, cred)
	case "stop":
		resp = h.handleStop(req, cred)
	case "restart":
		resp = h.handleRestart(req, cred)
	case "reload":
		resp = h.handleReload(req, cred)
	case "status":
		resp = h.handleStatus(cred)
	case "log":
		resp = h.handleLog(req, cred)
	case "ping":
		resp = &types.DaemonResponse{Success: true, Message: "pong"}
	default:
		resp = &types.DaemonResponse{
			Success: false,
			Message: fmt.Sprintf("Unknown action: %s", req.Action),
		}
	}

	socket.WriteResponse(conn, resp)
}

func (h *Handler) handleStart(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	_, svc, err := h.loadServiceConfig(req.Service, cred.UID)
	if err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}
	if err := h.validateServicePaths(req.Service, cred.UID, svc); err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	if err := h.Manager.Start(req.Service, *svc, cred.UID, cred.GID); err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	return &types.DaemonResponse{
		Success: true,
		Message: fmt.Sprintf("Service %q started successfully", req.Service),
	}
}

func (h *Handler) handleStop(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	if err := h.Manager.Stop(req.Service, cred.UID); err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	return &types.DaemonResponse{
		Success: true,
		Message: fmt.Sprintf("Service %q stopped", req.Service),
	}
}

func (h *Handler) handleRestart(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	_, svc, err := h.loadServiceConfig(req.Service, cred.UID)
	if err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}
	if err := h.validateServicePaths(req.Service, cred.UID, svc); err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	if err := h.Manager.Restart(req.Service, *svc, cred.UID, cred.GID); err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	return &types.DaemonResponse{
		Success: true,
		Message: fmt.Sprintf("Service %q restarted", req.Service),
	}
}

// handleReload re-reads ~/.pqpm.toml and restarts service(s) with the new config.
// req.Service must be a concrete name, or "*" for all services defined in the TOML.
func (h *Handler) handleReload(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	if req.Service == "" {
		return &types.DaemonResponse{
			Success: false,
			Message: "service name required (use explicit name or \"*\" for all)",
		}
	}

	u, err := user.LookupId(strconv.FormatUint(uint64(cred.UID), 10))
	if err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}
	cfg, err := config.LoadUserConfig(u.HomeDir)
	if err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	var names []string
	var notes []string
	if req.Service == "*" {
		for name := range cfg.Service {
			names = append(names, name)
		}
		// Drop runtime leftovers that are no longer in the TOML (avoids confusing "removed" ghosts).
		for _, running := range h.Manager.Status(cred.UID) {
			if _, ok := cfg.Service[running.Name]; ok {
				continue
			}
			if err := h.Manager.Stop(running.Name, cred.UID); err != nil {
				notes = append(notes, fmt.Sprintf("dropped %s (not in ~/.pqpm.toml): %v", running.Name, err))
			} else {
				notes = append(notes, fmt.Sprintf("dropped %s (not in ~/.pqpm.toml)", running.Name))
			}
		}
		if len(names) == 0 {
			msg := "No services defined in ~/.pqpm.toml"
			if len(notes) > 0 {
				msg += " (" + strings.Join(notes, "; ") + ")"
			}
			return &types.DaemonResponse{Success: true, Message: msg}
		}
	} else {
		names = []string{req.Service}
	}

	var okNames []string
	var errs []string
	for _, name := range names {
		svc, err := config.GetServiceConfig(cfg, name)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if err := config.ValidateServiceConfig(name, svc); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if err := h.validateServicePaths(name, cred.UID, svc); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if err := h.Manager.Restart(name, *svc, cred.UID, cred.GID); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		okNames = append(okNames, name)
	}

	if len(okNames) == 0 && len(errs) > 0 {
		msg := "Reload failed: " + strings.Join(errs, "; ")
		if len(notes) > 0 {
			msg += " (" + strings.Join(notes, "; ") + ")"
		}
		return &types.DaemonResponse{Success: false, Message: msg}
	}

	msg := fmt.Sprintf("Reloaded %s from ~/.pqpm.toml", strings.Join(okNames, ", "))
	if len(okNames) == 0 && len(notes) > 0 {
		msg = strings.Join(notes, "; ")
	} else if len(notes) > 0 {
		msg += " (" + strings.Join(notes, "; ") + ")"
	}
	if len(errs) > 0 {
		msg += " (errors: " + strings.Join(errs, "; ") + ")"
		return &types.DaemonResponse{Success: false, Message: msg}
	}
	return &types.DaemonResponse{Success: true, Message: msg}
}

func (h *Handler) handleStatus(cred *socket.PeerCred) *types.DaemonResponse {
	services := h.Manager.Status(cred.UID)
	return &types.DaemonResponse{
		Success:  true,
		Message:  fmt.Sprintf("Found %d service(s)", len(services)),
		Services: services,
	}
}

// handleLog returns the resolved log file path for the calling user to read/tail.
func (h *Handler) handleLog(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	if req.Service == "" {
		return &types.DaemonResponse{Success: false, Message: "service name is required"}
	}

	logPath := h.Manager.LogPath(req.Service, cred.UID)
	if _, svc, err := h.loadServiceConfig(req.Service, cred.UID); err == nil {
		if resolved, err := process.ResolveLogPath(req.Service, cred.UID, *svc); err == nil {
			logPath = resolved
		}
	} else if !h.Manager.HasService(req.Service, cred.UID) {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	return &types.DaemonResponse{
		Success: true,
		Message: fmt.Sprintf("Log file: %s", logPath),
		LogPath: logPath,
	}
}

func (h *Handler) validateServicePaths(name string, uid uint32, svc *types.ServiceConfig) error {
	if svc.WorkingDir != "" {
		if err := config.SanitizeUserPath(svc.WorkingDir, uid); err != nil {
			return fmt.Errorf("security violation: %w", err)
		}
	}
	if svc.LogFile != "" {
		logPath, err := process.ResolveLogPath(name, uid, *svc)
		if err != nil {
			return err
		}
		if err := config.SanitizeUserPath(logPath, uid); err != nil {
			return fmt.Errorf("security violation: %w", err)
		}
	}
	return nil
}

func (h *Handler) loadServiceConfig(serviceName string, uid uint32) (*types.UserConfig, *types.ServiceConfig, error) {
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to look up user for UID %d: %w", uid, err)
	}

	cfg, err := config.LoadUserConfig(u.HomeDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config for user %s: %w", u.Username, err)
	}

	svc, err := config.GetServiceConfig(cfg, serviceName)
	if err != nil {
		return nil, nil, err
	}

	if err := config.ValidateServiceConfig(serviceName, svc); err != nil {
		return nil, nil, err
	}

	return cfg, svc, nil
}
