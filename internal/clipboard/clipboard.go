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

// Package clipboard wraps the platform's native clipboard CLI tool.
// Pure-Go clipboard libraries either require CGO (Linux X11) or pull in
// large dependencies; the platform tools are already present on every
// system this CLI targets.
package clipboard

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
)

// ErrNoTool is returned when no usable clipboard tool is available.
var ErrNoTool = errors.New("no clipboard tool available")

// Copy writes value to the system clipboard. value can contain any bytes;
// the underlying tools accept binary data via stdin.
func Copy(ctx context.Context, value []byte) error {
	cmd, err := buildCommand(ctx)
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("clipboard stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("clipboard start: %w", err)
	}
	if _, err := stdin.Write(value); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return fmt.Errorf("clipboard write: %w", err)
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Wait()
		return fmt.Errorf("clipboard close: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("clipboard tool exited: %w", err)
	}
	return nil
}

// Clear empties the clipboard by copying a single newline. We deliberately
// don't copy an empty string — some tools (xclip) treat empty input as
// "do nothing" and leave the previous value behind.
func Clear(ctx context.Context) error {
	return Copy(ctx, []byte("\n"))
}

// buildCommand picks the right native tool for the current platform. We
// prefer Wayland's wl-copy when WAYLAND_DISPLAY is set, otherwise xclip
// for X11. Returns ErrNoTool with a hint when nothing usable is found.
func buildCommand(ctx context.Context) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		if path, err := exec.LookPath("pbcopy"); err == nil {
			return exec.CommandContext(ctx, path), nil
		}
	case "linux", "freebsd":
		if path, err := exec.LookPath("wl-copy"); err == nil {
			return exec.CommandContext(ctx, path), nil
		}
		if path, err := exec.LookPath("xclip"); err == nil {
			return exec.CommandContext(ctx, path, "-selection", "clipboard"), nil
		}
		if path, err := exec.LookPath("xsel"); err == nil {
			return exec.CommandContext(ctx, path, "--clipboard", "--input"), nil
		}
	}
	return nil, fmt.Errorf("%w: install one of pbcopy (macOS) / wl-copy / xclip / xsel", ErrNoTool)
}
