package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type conversionOptions struct {
	DeleteSource bool
	TrashDir     string
}
type fileProof struct {
	info os.FileInfo
	hash [32]byte
}
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func proveFile(ctx context.Context, path string) (fileProof, error) {
	f, e := os.Open(path)
	if e != nil {
		return fileProof{}, e
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		return fileProof{}, e
	}
	if !before.Mode().IsRegular() {
		return fileProof{}, fmt.Errorf("不是普通文件")
	}
	h := sha256.New()
	if _, e = io.Copy(h, contextReader{ctx, f}); e != nil {
		return fileProof{}, e
	}
	after, e := f.Stat()
	if e != nil {
		return fileProof{}, e
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return fileProof{}, fmt.Errorf("文件在读取期间变化")
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return fileProof{after, sum}, nil
}
func verifySavedOutput(ctx context.Context, staged, target string) error {
	want, e := proveFile(ctx, staged)
	if e != nil {
		return e
	}
	got, e := proveFile(ctx, target)
	if e != nil {
		return e
	}
	if want.info.Size() != got.info.Size() || want.hash != got.hash {
		return fmt.Errorf("目标文件读回校验不一致")
	}
	return nil
}

// Preserve an independently verified copy in Trash before unlinking the source.
func trashVerifiedSource(ctx context.Context, path string, before fileProof, trashDir string) (result error) {
	current, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !current.Mode().IsRegular() || !os.SameFile(before.info, current) {
		return fmt.Errorf("源文件身份变化，已保留")
	}
	if trashDir == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		trashDir = filepath.Join(home, ".Trash")
	}
	if e = os.MkdirAll(trashDir, 0700); e != nil {
		return e
	}
	reservation, e := os.CreateTemp(trashDir, filepath.Base(path)+".ncmdump-*")
	if e != nil {
		return e
	}
	trashPath := reservation.Name()
	reservation.Close()
	if e = os.Remove(trashPath); e != nil {
		return e
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(trashPath)
		}
	}()
	if e = publishOutput(ctx, path, trashPath); e != nil {
		return e
	}
	kept, e := proveFile(ctx, trashPath)
	if e != nil {
		return e
	}
	if kept.hash != before.hash || kept.info.Size() != before.info.Size() {
		return fmt.Errorf("废纸篓备份校验失败，源文件保留")
	}
	now, e := proveFile(ctx, path)
	if e != nil {
		return e
	}
	entry, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !entry.Mode().IsRegular() || !os.SameFile(before.info, entry) || !os.SameFile(before.info, now.info) || now.hash != before.hash || !now.info.ModTime().Equal(before.info.ModTime()) {
		return fmt.Errorf("源文件在转换期间变化，已保留")
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = os.Remove(path); e != nil {
		return e
	}
	committed = true
	return nil
}
