package common

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readOneRedisCommand(conn net.Conn) error {
	reader := bufio.NewReader(conn)
	header, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(header, "*") {
		return fmt.Errorf("unexpected RESP header %q", header)
	}
	fieldCount, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "*")))
	if err != nil {
		return err
	}
	for range fieldCount {
		lengthLine, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if !strings.HasPrefix(lengthLine, "$") {
			return fmt.Errorf("unexpected RESP bulk header %q", lengthLine)
		}
		length, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(lengthLine, "$")))
		if err != nil {
			return err
		}
		if _, err := io.CopyN(io.Discard, reader, int64(length+2)); err != nil {
			return err
		}
	}
	return nil
}

func executeMutationAgainstLostReply(t *testing.T, maxRetries int) int32 {
	t.Helper()
	var executions atomic.Int32
	client := redis.NewClient(&redis.Options{
		Addr:            "lost-reply.test:6379",
		MaxRetries:      maxRetries,
		MinRetryBackoff: -1,
		MaxRetryBackoff: -1,
		ReadTimeout:     time.Second,
		WriteTimeout:    time.Second,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go func() {
				defer serverConn.Close()
				if readOneRedisCommand(serverConn) == nil {
					// Model Redis applying the mutation, followed by a lost reply.
					executions.Add(1)
					// A truncated RESP integer produces io.ErrUnexpectedEOF, one of
					// the transport failures go-redis normally retries.
					_, _ = serverConn.Write([]byte(":"))
				}
			}()
			return clientConn, nil
		},
	})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	err := client.IncrBy(context.Background(), "quota", 1).Err()
	require.Error(t, err)
	return executions.Load()
}

func TestRedisOptionsDisableAutomaticCommandReplay(t *testing.T) {
	t.Setenv("REDIS_CONN_STRING", "redis://127.0.0.1:6379/0?max_retries=9")
	options := ParseRedisOption()
	require.Equal(t, -1, options.MaxRetries, "configured URL retries must be overridden for mutation safety")

	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	assert.Zero(t, client.Options().MaxRetries, "go-redis normalizes -1 to zero actual retries")
}

func TestLostRedisMutationReplyIsNotReplayed(t *testing.T) {
	assert.EqualValues(t, 1, executeMutationAgainstLostReply(t, -1),
		"a mutation with an unknown result must be sent exactly once")
	assert.EqualValues(t, 2, executeMutationAgainstLostReply(t, 1),
		"the fixture must detect go-redis replay when retries are enabled")
}
