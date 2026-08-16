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
	var lastRefKeys []string
	firstTick := true
	reporter := &driftReporter{}

	tick := func() error {
		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		resp, err := client.Resolve(reqCtx, &discoveryv1.ResolveRequest{Namespace: namespace})
		if err != nil {
			return pollFailure(out, "resolve poll failed", err)
		}

		resolved := resolvedMap(resp)
		data, err := yaml.Marshal(resolved)
		if err != nil {
			return pollFailure(out, "marshaling resolved config failed", err)
		}
		currentRefKeys := refKeys(resolved)

		if bytes.Equal(data, lastWritten) {
			firstTick = false
			return nil
		}

		if err := writeFileAtomic(out, data); err != nil {
			return pollFailure(out, "writing config file failed", err)
		}

		// Only after the baseline poll: the very first tick has nothing to
		// have drifted from, so nothing here is "newly appeared" yet.
		if !firstTick {
			if added := newlyAppearedRefs(lastRefKeys, currentRefKeys); len(added) > 0 {
				reporter.Report(ctx, namespace, added)
			}
		}

		lastWritten = data
		lastRefKeys = currentRefKeys
		firstTick = false
		return nil
	}

	// Write once immediately so the file exists before the first interval
	// elapses. pollFailure decides whether a failure here is fatal: on a cold
	// start there is no file to fall back on, but after a sidecar restart the
	// init container has already written one to the shared volume.
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
	return yaml.Marshal(resolvedMap(resp))
}

// resolvedMap flattens a ResolveResponse into the same map shape
// marshalConfigFile writes out - shared with drift.go's *_ref detection so
// both read the identical data, not two independent conversions of the
// same response that could quietly disagree.
func resolvedMap(resp *discoveryv1.ResolveResponse) map[string]any {
	out := make(map[string]any, len(resp.GetValues()))
	for _, kv := range resp.GetValues() {
		out[kv.GetKey()] = kv.GetValue().AsInterface()
	}
	return out
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

	// CreateTemp makes the file 0600 and rename preserves it, which leaves it
	// unreadable by a consumer container running as a different user - the
	// ordinary case, since the injected containers run as the image's own
	// non-root user. The file holds resolved configuration and never a secret
	// value (see docs/adr/0012-csi-secret-delivery.md), so it is world-readable
	// by design.
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("setting temp file mode: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file into place: %w", err)
	}

	return nil
}
