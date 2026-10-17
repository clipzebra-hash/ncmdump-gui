package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestExclusiveCopyPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	data := bytes.Repeat([]byte("audio"), 100000)
	os.WriteFile(source, data, 0600)
	if e := copyExclusive(context.Background(), source, target); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, data) {
		t.Fatal("content mismatch")
	}
	os.WriteFile(source, []byte("replacement"), 0600)
	if e := copyExclusive(context.Background(), source, target); !errors.Is(e, os.ErrExist) {
		t.Fatal(e)
	}
	got, _ = os.ReadFile(target)
	if !bytes.Equal(got, data) {
		t.Fatal("existing file changed")
	}
}

type cancelAfterChecks struct {
	context.Context
	checks atomic.Int32
}

func (c *cancelAfterChecks) Err() error {
	if c.checks.Add(1) > 3 {
		return context.Canceled
	}
	return nil
}
func TestExclusiveCopyCancelCleansPartial(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	os.WriteFile(source, make([]byte, 2*1024*1024), 0600)
	ctx := &cancelAfterChecks{Context: context.Background()}
	if e := copyExclusive(ctx, source, target); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := os.Stat(target); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("partial output retained", e)
	}
}
func TestSMBPublishAndReadback(t *testing.T) {
	base := os.Getenv("NCM_SMB_TEST_DIR")
	if base == "" {
		t.Skip("NCM_SMB_TEST_DIR required")
	}
	dir, e := os.MkdirTemp(base, ".ncmdump-regression-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	source := filepath.Join(t.TempDir(), "source")
	target := filepath.Join(dir, "output")
	data := bytes.Repeat([]byte("NCM offline publish verification\n"), 16384)
	os.WriteFile(source, data, 0600)
	if e = publishOutput(context.Background(), source, target); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(target)
	if e != nil || !bytes.Equal(got, data) {
		t.Fatal("SMB readback mismatch", e)
	}
	if e = publishOutput(context.Background(), source, target); !errors.Is(e, os.ErrExist) {
		t.Fatal("SMB failed to reject collision", e)
	}
	got, _ = os.ReadFile(target)
	if !bytes.Equal(got, data) {
		t.Fatal("SMB existing output modified")
	}
}

func TestSMBRealConversionReadback(t *testing.T) {
	base := os.Getenv("NCM_SMB_TEST_DIR")
	if base == "" {
		t.Skip("NCM_SMB_TEST_DIR required")
	}
	source := fixture(t)
	before, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	local := t.TempDir()
	remote, e := os.MkdirTemp(base, ".ncmdump-regression-real-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(remote)
	if _, e = convertFile(context.Background(), source, local); e != nil {
		t.Fatal(e)
	}
	if _, e = convertFile(context.Background(), source, remote); e != nil {
		t.Fatal(e)
	}
	files, _ := os.ReadDir(local)
	if len(files) != 1 {
		t.Fatal(files)
	}
	want, _ := os.ReadFile(filepath.Join(local, files[0].Name()))
	got, e := os.ReadFile(filepath.Join(remote, files[0].Name()))
	if e != nil || !bytes.Equal(normalizeAudio(got), normalizeAudio(want)) {
		t.Fatal("real SMB conversion differs from local", e)
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(before, after) {
		t.Fatal("NCM source changed")
	}
}
