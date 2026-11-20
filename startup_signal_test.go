//go:build !windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Block the real startup banner after HTTP starts, keeping the signal race
// deterministic without sleeps or a test hook in the production lifecycle.
type startupLogGate struct {
	ready, release *os.File
	once           sync.Once
}

func (g *startupLogGate) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "ready in ") {
		g.once.Do(func() {
			_, _ = g.ready.Write([]byte{1})
			var released [1]byte
			_, _ = io.ReadFull(g.release, released[:])
		})
	}
	return os.Stdout.Write(p)
}

func TestStartupSIGTERMAfterHTTPReadyDrainsGracefully(t *testing.T) {
	if os.Getenv("LEMONHUB_TEST_STARTUP_SIGNAL") == "1" {
		ready, release := os.NewFile(3, "startup-ready"), os.NewFile(4, "startup-release")
		defer ready.Close()
		defer release.Close()
		*common.LogDir = ""
		gin.DefaultWriter = &startupLogGate{ready: ready, release: release}
		main()
		return
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	readyRead, readyWrite, err := os.Pipe()
	require.NoError(t, err)
	defer readyRead.Close()
	defer readyWrite.Close()
	releaseRead, releaseWrite, err := os.Pipe()
	require.NoError(t, err)
	defer releaseRead.Close()
	defer releaseWrite.Close()
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStartupSIGTERMAfterHTTPReadyDrainsGracefully$", "-test.v", "-log-dir=", "-port="+fmt.Sprint(port))
	cmd.Dir = dir
	// Never inherit a developer/production DSN or background-service settings.
	cmd.Env = []string{
		"LEMONHUB_TEST_STARTUP_SIGNAL=1",
		"SQLITE_PATH=" + filepath.Join(dir, "startup.db"),
		"BIND_ADDRESS=127.0.0.1",
		"UPDATE_TASK=false",
		"SESSION_SECRET=startup-signal-test-only-secret",
	}
	cmd.ExtraFiles = []*os.File{readyWrite, releaseRead}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	require.NoError(t, cmd.Start())
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	require.NoError(t, readyWrite.Close())
	require.NoError(t, releaseRead.Close())
	ready := make(chan error, 1)
	go func() {
		var marker [1]byte
		_, err := io.ReadFull(readyRead, marker[:])
		ready <- err
	}()
	select {
	case err := <-ready:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("application did not reach the startup banner")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status", port))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	_, _ = releaseWrite.Write([]byte{1})
	err = cmd.Wait()
	waited = true
	require.NoError(t, err, "%s", output.String())
	assert.Contains(t, output.String(), "received signal: terminated, shutting down...")
	assert.Contains(t, output.String(), "server exited")
}
