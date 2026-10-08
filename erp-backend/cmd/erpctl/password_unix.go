//go:build darwin || linux

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func terminalPassword(ctx context.Context, prompt io.Writer) (password string, err error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", errors.New("no terminal; use --password-stdin")
	}
	defer func() {
		if closeErr := tty.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close terminal: %w", closeErr))
		}
	}()
	fd := int(tty.Fd())
	state, err := unix.IoctlGetTermios(fd, terminalGet)
	if err != nil {
		return "", fmt.Errorf("read terminal state: %w", err)
	}
	hidden := *state
	hidden.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(fd, terminalSet, &hidden); err != nil {
		return "", fmt.Errorf("hide password: %w", err)
	}
	defer func() {
		restoreErr := unix.IoctlSetTermios(fd, terminalSet, state)
		if restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore terminal: %w", restoreErr))
		}
	}()
	if _, err := fmt.Fprint(prompt, "管理员密码: "); err != nil {
		return "", err
	}
	type result struct {
		line string
		err  error
	}
	ready := make(chan result, 1)
	go func() {
		line, e := bufio.NewReader(io.LimitReader(tty, 4097)).ReadString('\n')
		ready <- result{line: line, err: e}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-ready:
		if value.err != nil {
			return "", fmt.Errorf("read password: %w", value.err)
		}
		if _, err := fmt.Fprintln(prompt); err != nil {
			return "", err
		}
		password = strings.TrimSuffix(strings.TrimSuffix(value.line, "\n"), "\r")
		if password == "" {
			return "", errors.New("password is empty")
		}
		return password, nil
	}
}
