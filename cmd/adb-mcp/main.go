// Command adb-mcp is an MCP server for Android that drives emulators/devices
// over adb: boot AVDs, screenshot, read the UI hierarchy, tap/swipe/type,
// manage the device lock, read logcat, and control app lifecycle. It is the
// Android counterpart to XcodeBuildMCP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/iksnerd/adb_mcp/internal/adb"
	"github.com/iksnerd/adb_mcp/internal/guides"
	"github.com/iksnerd/adb_mcp/internal/selfupdate"
	"github.com/iksnerd/adb_mcp/internal/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// version is overridable at build time via -ldflags "-X main.version=...".
// The Makefile injects the value from the VERSION file / git.
var version = "0.14.0"

func main() {
	log.SetFlags(0)
	log.SetPrefix("adb-mcp: ")

	// Subcommands come before flag parsing: `adb-mcp update` / `adb-mcp version`.
	// Anything else falls through to the default mode — serving MCP over stdio.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "update":
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := selfupdate.Run(ctx, version, os.Stdout); err != nil {
				log.Fatalf("update failed: %v", err)
			}
			return
		case "version":
			fmt.Printf("adb-mcp %s\n", version)
			return
		}
	}

	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("adb-mcp %s\n", version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "adb-mcp",
		Version: version,
	}, nil)

	tools.ServerVersion = version
	tools.Register(srv)
	guides.Register(srv)

	err := srv.Run(ctx, &mcp.StdioTransport{})

	// Tear down any running logcat/screen-record capture sessions on EVERY exit
	// path so their detached adb processes and temp files don't leak. This runs
	// explicitly (not via defer) because the log.Fatalf below would os.Exit and
	// skip deferred cleanup on the error path.
	adb.StopAllCaptures()

	// A cancelled context (SIGINT/SIGTERM) or a closed stdin (the MCP client
	// disconnecting) is a normal shutdown, not a failure — exit 0 quietly.
	// Only a genuinely unexpected error is fatal.
	if err != nil && ctx.Err() == nil && !isCleanShutdown(err) {
		log.Fatalf("server error: %v", err)
	}
}

// isCleanShutdown reports whether a non-nil error from srv.Run just means the
// stdio stream closed (the MCP client went away), which for a stdio server is a
// normal end-of-life, not a crash. We check io.EOF / a closed pipe first; the
// go-sdk (v1.6.1) unfortunately folds the underlying io.EOF into a JSON-RPC
// "server is closing" WireError *string* with no exported sentinel to match, so
// we fall back to that documented message.
func isCleanShutdown(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "server is closing") || strings.HasSuffix(msg, "EOF")
}
