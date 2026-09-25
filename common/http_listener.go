package common

import (
	"net"
	"os"
)

// HTTPListenAddress preserves the gateway's public default while allowing local
// clients, including the desktop application, to bind only to loopback.
func HTTPListenAddress(port string) string {
	return net.JoinHostPort(os.Getenv("BIND_ADDRESS"), port)
}

// PprofListenAddress keeps unauthenticated diagnostics local unless an operator
// explicitly configures another listener behind their own access controls.
func PprofListenAddress() string {
	return GetEnvOrDefaultString("PPROF_LISTEN_ADDRESS", "127.0.0.1:8005")
}
