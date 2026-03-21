//go:build windows

package ptymanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/UserExistsError/conpty"
)

// windowsPTY wraps ConPTY for Windows systems.
type windowsPTY struct {
	cpty *conpty.ConPty
	done chan struct{}
}

func startPTYProcess(name string, args []string, dir string, env []string, cols, rows uint16) (ptyProcess, error) {
	// ConPTY takes a single command line string.
	// Prepend a cd to set working directory since conpty.Start doesn't support WorkDir directly.
	cmdLine := name
	if len(args) > 0 {
		cmdLine = name + " " + strings.Join(args, " ")
	}
	if dir != "" {
		cmdLine = fmt.Sprintf(`cd /d "%s" && %s`, dir, cmdLine)
	}

	cpty, err := conpty.Start(cmdLine, conpty.ConPtyDimensions(int(cols), int(rows)))
	if err != nil {
		return nil, fmt.Errorf("conpty.Start: %w", err)
	}

	p := &windowsPTY{
		cpty: cpty,
		done: make(chan struct{}),
	}

	go func() {
		p.cpty.Wait(context.Background())
		close(p.done)
	}()

	return p, nil
}

func (p *windowsPTY) Read(b []byte) (int, error)  { return p.cpty.Read(b) }
func (p *windowsPTY) Write(b []byte) (int, error) { return p.cpty.Write(b) }
func (p *windowsPTY) Done() <-chan struct{}        { return p.done }

func (p *windowsPTY) Resize(cols, rows uint16) error {
	return p.cpty.Resize(int(cols), int(rows))
}

func (p *windowsPTY) Terminate() error {
	return p.cpty.Close()
}

func (p *windowsPTY) ForceKill() error {
	return p.cpty.Close()
}

func (p *windowsPTY) Close() error {
	return p.cpty.Close()
}

// shellWrap wraps a command string for execution via the Windows shell.
func shellWrap(cmd string) []string {
	return []string{"cmd.exe", "/c", cmd}
}
