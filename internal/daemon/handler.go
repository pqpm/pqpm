package daemon

import (
	"fmt"
	"net"
	"os/user"
	"strconv"

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

	// Get peer credentials for identity validation
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

	// Read the request
	req, err := socket.ReadRequest(conn)
	if err != nil {
		logger.Log.Warn("Failed to read request", "error", err)
		socket.WriteResponse(conn, &types.DaemonResponse{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	// Dispatch based on action
	var resp *types.DaemonResponse
	switch req.Action {
	case "start":
		resp = h.handleStart(req, cred)
	case "stop":
		resp = h.handleStop(req, cred)
	case "restart":
		resp = h.handleRestart(req, cred)
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

// handleStart loads the user's config and starts the named service.
func (h *Handler) handleStart(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	_, svc, err := h.loadServiceConfig(req.Service, cred.UID)
	if err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	// Security: Ensure working directory is within user's home
	if svc.WorkingDir != "" {
		if err := config.SanitizeUserPath(svc.WorkingDir, cred.UID); err != nil {
			return &types.DaemonResponse{Success: false, Message: "Security violation: " + err.Error()}
		}
	}

	if err := h.Manager.Start(req.Service, *svc, cred.UID, cred.GID); err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	return &types.DaemonResponse{
		Success: true,
		Message: fmt.Sprintf("Service %q started successfully", req.Service),
	}
}

// handleStop stops the named service for the user.
func (h *Handler) handleStop(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	if err := h.Manager.Stop(req.Service, cred.UID); err != nil {
		return &types.DaemonResponse{Success: false, Message: err.Error()}
	}

	return &types.DaemonResponse{
		Success: true,
		Message: fmt.Sprintf("Service %q stopped", req.Service),
	}
}

// handleRestart reloads config and restarts the service.
func (h *Handler) handleRestart(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	_, svc, err := h.loadServiceConfig(req.Service, cred.UID)
	if err != nil {
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

// handleStatus returns all processes for the requesting user.
func (h *Handler) handleStatus(cred *socket.PeerCred) *types.DaemonResponse {
	services := h.Manager.Status(cred.UID)
	return &types.DaemonResponse{
		Success:  true,
		Message:  fmt.Sprintf("Found %d service(s)", len(services)),
		Services: services,
	}
}

// handleLog returns the log file path for the service (placeholder for now).
func (h *Handler) handleLog(req *types.DaemonRequest, cred *socket.PeerCred) *types.DaemonResponse {
	logPath := fmt.Sprintf("/var/log/pqpm/users/%d/%s.log", cred.UID, req.Service)
	return &types.DaemonResponse{
		Success: true,
		Message: fmt.Sprintf("Log file: %s", logPath),
	}
}

// loadServiceConfig looks up the user's home directory, loads their .pqpm.toml,
// and returns the config for the requested service.
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
