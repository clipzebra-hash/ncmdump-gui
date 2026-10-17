package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func disposableNCM(t *testing.T) string {
	t.Helper()
	data, e := os.ReadFile(fixture(t))
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "test.ncm")
	if e = os.WriteFile(p, data, 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestDeleteOptionSuccessMovesVerifiedSourceToTrash(t *testing.T) {
	source := disposableNCM(t)
	before, _ := os.ReadFile(source)
	out := t.TempDir()
	trash := t.TempDir()
	if _, e := convertFileWithOptions(context.Background(), source, out, conversionOptions{true, trash}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(source); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("source still present", e)
	}
	entries, _ := os.ReadDir(trash)
	if len(entries) != 1 {
		t.Fatal(entries)
	}
	got, _ := os.ReadFile(filepath.Join(trash, entries[0].Name()))
	if !bytes.Equal(got, before) {
		t.Fatal("Trash copy differs")
	}
	outputs, _ := os.ReadDir(out)
	if len(outputs) != 1 {
		t.Fatal(outputs)
	}
}
func TestDeleteOptionExistingOutputKeepsSource(t *testing.T) {
	source := disposableNCM(t)
	out := t.TempDir()
	trash := t.TempDir()
	if _, e := convertFile(context.Background(), source, out); e != nil {
		t.Fatal(e)
	}
	if _, e := convertFileWithOptions(context.Background(), source, out, conversionOptions{true, trash}); e == nil {
		t.Fatal("expected collision")
	}
	if _, e := os.Stat(source); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(trash)
	if len(entries) != 0 {
		t.Fatal("moved source despite collision")
	}
}
func TestDeleteOptionFailureAndCancellationKeepSource(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		source := filepath.Join(t.TempDir(), "invalid.ncm")
		os.WriteFile(source, []byte("invalid"), 0600)
		trash := t.TempDir()
		ctx, stop := context.WithCancel(context.Background())
		if cancel {
			stop()
		}
		_, e := convertFileWithOptions(ctx, source, t.TempDir(), conversionOptions{true, trash})
		stop()
		if e == nil {
			t.Fatal("expected failure")
		}
		if _, e = os.Stat(source); e != nil {
			t.Fatal(e)
		}
	}
}
func TestReadbackMismatchAndSourceChangeRefuseDeletion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	os.WriteFile(a, []byte("before"), 0600)
	os.WriteFile(b, []byte("wrong"), 0600)
	if e := verifySavedOutput(ctx, a, b); e == nil {
		t.Fatal("accepted mismatch")
	}
	before, e := proveFile(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(a, []byte("changed"), 0600)
	trash := t.TempDir()
	if e = trashVerifiedSource(ctx, a, before, trash); e == nil {
		t.Fatal("deleted changed source")
	}
	got, _ := os.ReadFile(a)
	if string(got) != "changed" {
		t.Fatal("source not retained")
	}
	entries, _ := os.ReadDir(trash)
	if len(entries) != 0 {
		t.Fatal(entries)
	}
}
func TestTrashFailureKeepsSourceAndConvertedOutput(t *testing.T) {
	source := disposableNCM(t)
	out := t.TempDir()
	notDir := filepath.Join(t.TempDir(), "file")
	os.WriteFile(notDir, []byte("block"), 0600)
	if _, e := convertFileWithOptions(context.Background(), source, out, conversionOptions{true, notDir}); e == nil {
		t.Fatal("expected cleanup failure")
	}
	if _, e := os.Stat(source); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 1 {
		t.Fatal("converted output not retained")
	}
}
func TestDeleteOptionSMBReadback(t *testing.T) {
	base := os.Getenv("NCM_SMB_TEST_DIR")
	if base == "" {
		t.Skip("NCM_SMB_TEST_DIR required")
	}
	source := disposableNCM(t)
	out, e := os.MkdirTemp(base, ".ncmdump-delete-regression-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(out)
	trash := t.TempDir()
	if _, e = convertFileWithOptions(context.Background(), source, out, conversionOptions{true, trash}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(source); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("source retained", e)
	}
	entries, _ := os.ReadDir(trash)
	if len(entries) != 1 {
		t.Fatal(entries)
	}
}
