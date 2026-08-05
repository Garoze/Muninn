package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"github.com/garoze/muninn/internal/discoveryclient"
	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"
)

const defaultResolveAddr = "localhost:5010"

func cmdResolve(args []string) error {
	fs := pflag.NewFlagSet("resolve", pflag.ExitOnError)
	addr := fs.StringP("addr", "a", defaultResolveAddr, "muninn gRPC address (host:port)")
	namespace := fs.StringP("namespace", "n", "", "namespace (required)")
	out := fs.StringP("out", "o", "", "output file path (required)")
	watch := fs.BoolP("watch", "w", false, "keep running and rewrite the file on drift instead of exiting after one write")
	interval := fs.DurationP("interval", "i", 15*time.Second, "poll interval when --watch is set")
	tlsCA := fs.StringP("tls-ca", "c", "", "path to a CA certificate for verifying the server's TLS certificate (unset = plaintext)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *namespace == "" || *out == "" {
		return fmt.Errorf("--namespace and --out are required (see 'muninn resolve -h')")
	}

	client, conn, err := discoveryclient.Dial(*addr, *tlsCA)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := conn.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "[muninn] warning: closing connection: %v\n", cerr)
		}
	}()

	if !*watch {
		return resolveOnce(context.Background(), client, *namespace, *out)
	}

	return resolveWatch(client, *namespace, *out, *interval)
}

// resolveOnce performs a single Resolve call and writes the result to out
// unconditionally.
func resolveOnce(ctx context.Context, client discoveryv1.DiscoveryServiceClient, namespace, out string) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.Resolve(reqCtx, &discoveryv1.ResolveRequest{Namespace: namespace})
	if err != nil {
		return err
	}

	data, err := marshalConfigFile(resp)
	if err != nil {
		return err
	}

	return writeFileAtomic(out, data)
}

// resolveWatch polls Resolve on interval and rewrites out only when the data
// changed since the last write.
//
// A poll failure is fatal only if out doesn't exist yet; once a file
// exists, a failure is logged and skipped so a stale-but-usable file stays
// in place rather than failing on one bad poll.
func resolveWatch(client discoveryv1.DiscoveryServiceClient, namespace, out string, interval time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var lastWritten []byte

	tick := func() error {
		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		resp, err := client.Resolve(reqCtx, &discoveryv1.ResolveRequest{Namespace: namespace})
		if err != nil {
			return pollFailure(out, "resolve poll failed", err)
		}

		data, err := marshalConfigFile(resp)
		if err != nil {
			return pollFailure(out, "marshaling resolved config failed", err)
		}

		if bytes.Equal(data, lastWritten) {
			return nil
		}

		if err := writeFileAtomic(out, data); err != nil {
			return pollFailure(out, "writing config file failed", err)
		}

		lastWritten = data
		return nil
	}

	// Write once immediately so the file exists before the first interval
	// elapses. A failure here is fatal by definition - out can't already
	// exist on a fresh start (pollFailure would still confirm that).
	if err := tick(); err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := tick(); err != nil {
				return err
			}
		}
	}
}

// pollFailure returns nil (recoverable, logged) if out already exists, or
// an error if there's no fallback file yet.
func pollFailure(out, msg string, err error) error {
	if _, statErr := os.Stat(out); statErr == nil {
		fmt.Fprintf(os.Stderr, "[muninn] warning: %s: %v\n", msg, err)
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func marshalConfigFile(resp *discoveryv1.ResolveResponse) ([]byte, error) {
	out := make(map[string]any, len(resp.GetValues()))
	for _, kv := range resp.GetValues() {
		out[kv.GetKey()] = kv.GetValue().AsInterface()
	}

	return yaml.Marshal(out)
}

// writeFileAtomic writes data to a temp file in the same directory as path,
// then renames it into place. rename(2) is atomic on the same filesystem,
// so a reader of path never observes a partially-written file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".muninn-config-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	tmpPath := tmp.Name()

	// Close/Remove errors on these cleanup paths are deliberately discarded:
	// the write/close/rename attempt that triggered the cleanup already
	// returns the more relevant error.
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file into place: %w", err)
	}

	return nil
}
