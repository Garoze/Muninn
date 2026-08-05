package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/pflag"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"github.com/garoze/muninn/internal/discoveryclient"
)

const defaultAddr = "localhost:5010"

// printUsage writes the top-level command overview to w. The write error is
// deliberately discarded: this is help text printed immediately before the
// program exits or returns, so a failed write (e.g. closed stdout pipe)
// isn't something the CLI could act on differently.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `muninnctl is a CLI client for Muninn's gRPC Query API.

Commands:
  query      Query configuration values for a namespace
  describe   List the active configuration sources

Usage:
  muninnctl [flags] [command]

Use "muninnctl <command> --help" for more information about a given command.
`)
}

// setFlagUsage overrides fs's default help output with kubectl's own leaf
// command layout: a short description, the "-x, --name" flag listing (via
// pflag's FlagUsages, the same formatting kubectl itself uses), and a
// trailing "Usage:" line. Write error discarded for the same reason as
// printUsage above.
func setFlagUsage(fs *pflag.FlagSet, name, desc string) {
	fs.Usage = func() {
		w := fs.Output()
		_, _ = fmt.Fprintf(w, "%s\n\nFlags:\n%s\nUsage:\n  muninnctl %s [flags]\n", desc, fs.FlagUsages(), name)
	}
}

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return
	case "query":
		err = cmdQuery(os.Args[2:])
	case "describe":
		err = cmdDescribe(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "[muninnctl] unknown command: %s\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "[muninnctl] %v\n", err)
		os.Exit(1)
	}
}

func cmdQuery(args []string) error {
	fs := pflag.NewFlagSet("query", pflag.ExitOnError)
	setFlagUsage(fs, "query", "Query configuration values for a namespace.")
	addr := fs.StringP("addr", "a", defaultAddr, "muninn gRPC address (host:port)")
	namespace := fs.StringP("namespace", "n", "", "namespace (required)")
	keys := fs.StringP("keys", "k", "", "comma-separated keys (required)")
	strict := fs.BoolP("strict", "s", false, "return InvalidArgument if any key is missing")
	tlsCA := fs.StringP("tls-ca", "c", "", "path to a CA certificate for verifying the server's TLS certificate (unset = plaintext)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *namespace == "" || *keys == "" {
		return fmt.Errorf("--namespace and --keys are required (see 'muninnctl query -h')")
	}

	client, conn, err := discoveryclient.Dial(*addr, *tlsCA)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "[muninnctl] warning: closing connection: %v\n", cerr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Query(ctx, &discoveryv1.QueryRequest{
		Namespace: *namespace,
		Keys:      strings.Split(*keys, ","),
		Strict:    *strict,
	})

	if err != nil {
		return err
	}

	return formatQueryResponse(os.Stdout, resp)
}

func cmdDescribe(args []string) error {
	fs := pflag.NewFlagSet("describe", pflag.ExitOnError)
	setFlagUsage(fs, "describe", "List the active configuration sources.")
	addr := fs.StringP("addr", "a", defaultAddr, "muninn gRPC address (host:port)")
	tlsCA := fs.StringP("tls-ca", "c", "", "path to a CA certificate for verifying the server's TLS certificate (unset = plaintext)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, conn, err := discoveryclient.Dial(*addr, *tlsCA)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "[muninnctl] warning: closing connection: %v\n", cerr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Describe(ctx, &discoveryv1.DescribeRequest{})
	if err != nil {
		return err
	}

	return formatDescribeResponse(os.Stdout, resp)
}

// formatQueryResponse writes query results as tab-aligned columns to w.
// Extracted so it can be unit-tested without a live server.
//
// Writes into tw are deliberately unchecked: tabwriter buffers them
// internally and only performs real I/O on Flush, so the first genuine
// write failure surfaces there (or on the direct write to w below), not on
// any individual buffered call.
func formatQueryResponse(w io.Writer, resp *discoveryv1.QueryResponse) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tVALUE\tSOURCE")
	for _, kv := range resp.GetValues() {
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\n",
			kv.GetKey(),
			kv.GetValue().AsInterface(),
			kv.GetSource(),
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if len(resp.GetMissingKeys()) > 0 {
		if _, err := fmt.Fprintf(w, "\nmissing: %s\n", strings.Join(resp.GetMissingKeys(), ", ")); err != nil {
			return err
		}
	}
	return nil
}

// formatDescribeResponse writes active config sources as tab-aligned columns
// to w. Extracted so it can be unit-tested without a live server. See
// formatQueryResponse for why writes into tw are unchecked.
func formatDescribeResponse(w io.Writer, resp *discoveryv1.DescribeResponse) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KIND\tLABEL SELECTOR\tSCOPE")
	for _, s := range resp.GetSources() {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n",
			s.GetKind(),
			s.GetLabelSelector(),
			s.GetScope(),
		)
	}
	return tw.Flush()
}
