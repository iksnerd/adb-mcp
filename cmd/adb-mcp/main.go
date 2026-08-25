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

	"github.com/iksnerd/adb-mcp/internal/adb"
	"github.com/iksnerd/adb-mcp/internal/bridgeupdate"
	"github.com/iksnerd/adb-mcp/internal/guides"
	"github.com/iksnerd/adb-mcp/internal/selfupdate"
	"github.com/iksnerd/adb-mcp/internal/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// version is overridable at build time via -ldflags "-X main.version=...".
// The Makefile injects the value from the VERSION file / git.
var version = "0.23.0"

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
		case "bridge":
			// EXPERIMENTAL: one-time per-device setup for the accessibility-click
			// bridge (tap_on_text/tap_element's via_accessibility=true). See
			// bridge/README.md.
			if len(os.Args) < 3 || os.Args[2] != "install" {
				log.Fatalf("usage: adb-mcp bridge install [--serial=<serial>]")
			}
			fs := flag.NewFlagSet("bridge install", flag.ExitOnError)
			serial := fs.String("serial", "", "target device serial (adb -s); optional when exactly one device is attached")
			_ = fs.Parse(os.Args[3:])
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := bridgeupdate.Run(ctx, *serial, os.Stdout); err != nil {
				log.Fatalf("bridge install failed: %v", err)
			}
			return
		}
	}

	showVersion := flag.Bool("version", false, "print version and exit")
	// An MCP client launches this server itself, often with a minimal
	// environment that has no ANDROID_HOME — and a client config may have
	// nowhere convenient to set one. Exporting it here means every later
	// sdk.Root() lookup, and the Gradle subprocesses that inherit this
	// environment, agree on the same SDK without threading a path through.
	sdkPath := flag.String("sdk", "", "Android SDK location (overrides ANDROID_HOME for this process)")
	flag.Parse()
	if *showVersion {
		fmt.Printf("adb-mcp %s\n", version)
		return
	}
	if *sdkPath != "" {
		if err := os.Setenv("ANDROID_HOME", *sdkPath); err != nil {
			log.Fatalf("could not apply --sdk: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Instructions ride along on initialize and land in the model's system
	// prompt before any tool is called — the one channel that doesn't depend on
	// the agent deciding to read something. See internal/tools/instructions.go.
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "adb-mcp",
		Version: version,
	}, &mcp.ServerOptions{Instructions: tools.Instructions})

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
