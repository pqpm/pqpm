package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pqpm/pqpm/addons/ui/internal/rpc"
	"github.com/pqpm/pqpm/addons/ui/internal/server"
	"github.com/pqpm/pqpm/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "rpc" {
		os.Exit(rpc.Run(os.Args[2:]))
	}

	fs := flag.NewFlagSet("pqpm-ui", flag.ExitOnError)
	addr := fs.String("listen", envOr("PQPM_UI_LISTEN", "127.0.0.1:9090"), "HTTP listen address")
	authMode := fs.String("auth", envOr("PQPM_UI_AUTH", "login"), "auth mode: login (Linux password) or local (current user, no password)")
	userName := fs.String("user", envOr("PQPM_UI_USER", ""), "fixed username for local auth (default: current user)")
	pqpmPath := fs.String("pqpm", envOr("PQPM_BIN", "pqpm"), "path to pqpm CLI")
	secure := fs.Bool("secure-cookie", false, "set Secure flag on session cookies (HTTPS)")
	showVersion := fs.Bool("version", false, "print version and exit")
	_ = fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pqpm-ui: %v\n", err)
		os.Exit(1)
	}

	srv, err := server.New(server.Config{
		Addr:         *addr,
		AuthMode:     *authMode,
		FixedUser:    *userName,
		PQPMPath:     *pqpmPath,
		SelfPath:     self,
		SecureCookie: *secure,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pqpm-ui: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "pqpm-ui listening on http://%s (auth=%s)\n", *addr, *authMode)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "pqpm-ui: %v\n", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
