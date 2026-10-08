//go:build !darwin && !linux

package main

import (
	"context"
	"errors"
	"io"
)

func terminalPassword(_ context.Context, _ io.Writer) (string, error) {
	return "", errors.New("interactive login requires macOS or Linux; use --password-stdin")
}
