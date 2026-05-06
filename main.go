/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/jtprogru/jtsekret/cmd"
	_ "github.com/jtprogru/jtsekret/internal/backend/file"
	_ "github.com/jtprogru/jtsekret/internal/backend/githubrepo"
	_ "github.com/jtprogru/jtsekret/internal/backend/lockbox"
	_ "github.com/jtprogru/jtsekret/internal/backend/vault"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func init() {
	cmd.Version = version
	cmd.Commit = commit
	cmd.BuildTime = buildTime
}

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Default to Warn so non-interactive subcommands (completion, exec piped
	// into another process) don't pollute stderr with startup noise. The
	// level is bumped to Debug by --debug or to whatever cfg.Log.Level says
	// inside PersistentPreRunE, once flags and config have been parsed.
	cmd.LogLevel.Set(slog.LevelWarn)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: &cmd.LogLevel,
	})))

	slog.Debug("starting jtsekret",
		slog.String("version", version),
		slog.String("commit", commit),
		slog.String("runtime", runtime.Version()),
	)

	if err := cmd.ExecuteContext(ctx); err != nil {
		slog.Error("execution failed", slog.Any("error", err))
		return 1
	}
	return 0
}
