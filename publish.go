package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

// Prefer a single-step publish on supporting volumes. SMB/exFAT may reject
// links: exclusive creation still protects all existing files from overwrite.
func publishOutput(ctx context.Context, source, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Link(source, target)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) && !errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.EPERM) && !errors.Is(err, syscall.ENOSYS) {
		return err
	}
	return copyExclusive(ctx, source, target)
}

func copyExclusive(ctx context.Context, source, target string) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		output.Close()
		if !committed {
			if e := os.Remove(target); e != nil && !errors.Is(e, os.ErrNotExist) {
				result = fmt.Errorf("清理未完成输出失败 %s: %v；原始错误: %v", target, e, result)
			}
		}
	}()
	buffer := make([]byte, 256*1024)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := input.Read(buffer)
		if n > 0 {
			written, e := output.Write(buffer[:n])
			if e != nil {
				return e
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = output.Sync(); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	committed = true
	return nil
}
