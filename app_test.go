package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	p := os.Getenv("NCM_TEST_FILE")
	if p == "" {
		return syntheticFixture(t)
	}
	return p
}
func waitBatch(t *testing.T, a *App) BatchProgress {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		p := a.GetProgress()
		if !p.Running {
			return p
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("batch did not finish")
	return BatchProgress{}
}
func TestConversionPreservesInputAndOutput(t *testing.T) {
	p := fixture(t)
	before, _ := os.ReadFile(p)
	dir := t.TempDir()
	if _, e := convertFile(context.Background(), p, dir); e != nil {
		t.Fatal(e)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	audio, _ := os.ReadFile(files[0])
	if _, e := convertFile(context.Background(), p, dir); e == nil {
		t.Fatal("overwrote output")
	}
	after, _ := os.ReadFile(p)
	audio2, _ := os.ReadFile(files[0])
	if sha256.Sum256(before) != sha256.Sum256(after) || sha256.Sum256(audio) != sha256.Sum256(audio2) {
		t.Fatal("changed existing file")
	}
}
func TestBatchDedupeFailureAndBusy(t *testing.T) {
	p := fixture(t)
	dir := t.TempDir()
	bad := filepath.Join(t.TempDir(), "bad.ncm")
	os.WriteFile(bad, []byte("invalid"), 0600)
	a := NewApp()
	files := []NcmFile{{p, Pending}, {p, Pending}, {bad, Pending}}
	if e := a.ProcessFiles(files, dir, false); e != nil {
		t.Fatal(e)
	}
	if e := a.ProcessFiles(files, dir, false); e == nil {
		t.Fatal("accepted concurrent batch")
	}
	r := waitBatch(t, a)
	resolvedBad, _ := filepath.EvalSymlinks(bad)
	if len(r.Errors) != 1 || r.Errors[0].File != resolvedBad || r.Errors[0].Message == "" {
		t.Fatal("missing per-file error detail", r)
	}
	if r.Total != 2 || r.Completed != 2 || r.Success != 1 || r.Failed != 1 {
		t.Fatal(r)
	}
}
func TestCancellation(t *testing.T) {
	p := fixture(t)
	a := NewApp()
	dir := t.TempDir()
	if e := a.ProcessFiles([]NcmFile{{p, Pending}}, dir, false); e != nil {
		t.Fatal(e)
	}
	a.CancelBatch()
	r := waitBatch(t, a)
	if !r.Cancelled || r.Completed != 0 {
		t.Fatal(r)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("partial output leaked")
	}
}

func TestCancelDuringWorkAndResponsiveProgress(t *testing.T) {
	p := fixture(t)
	dir := t.TempDir()
	jobs := []NcmFile{}
	for i := 0; i < 12; i++ {
		copyPath := filepath.Join(t.TempDir(), filepath.Base(p))
		data, _ := os.ReadFile(p)
		if e := os.WriteFile(copyPath, data, 0600); e != nil {
			t.Fatal(e)
		}
		jobs = append(jobs, NcmFile{copyPath, Pending})
	}
	a := NewApp()
	if e := a.ProcessFiles(jobs, dir, false); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Second)
	for a.GetProgress().Current == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	started := time.Now()
	a.GetProgress()
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("progress blocked")
	}
	started = time.Now()
	a.CancelBatch()
	r := waitBatch(t, a)
	t.Logf("cancel latency: %s; completed %d/%d", time.Since(started), r.Completed, r.Total)
	if !r.Cancelled || r.Completed >= r.Total {
		t.Fatal(r)
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatal("staging directory leaked")
		}
	}
}

func TestRealBatchProgress(t *testing.T) {
	base := os.Getenv("NCM_TEST_DIR")
	if base == "" {
		t.Skip("NCM_TEST_DIR required")
	}
	files, _ := filepath.Glob(filepath.Join(base, "*.ncm"))
	if len(files) > 12 {
		files = files[:12]
	}
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	jobs := []NcmFile{}
	for _, p := range files {
		jobs = append(jobs, NcmFile{p, Pending})
	}
	a := NewApp()
	out := t.TempDir()
	start := time.Now()
	if e := a.ProcessFiles(jobs, out, false); e != nil {
		t.Fatal(e)
	}
	last := 0
	for a.GetProgress().Running {
		p := a.GetProgress()
		if p.Completed < last || p.Completed != p.Success+p.Failed {
			t.Fatal(p)
		}
		last = p.Completed
		time.Sleep(time.Millisecond)
	}
	p := a.GetProgress()
	if p.Success != len(jobs) || p.Failed != 0 || p.Completed != p.Total {
		t.Fatal(p)
	}
	t.Logf("real batch %d files: %s", len(jobs), time.Since(start))
	baseline := os.Getenv("NCM_BASELINE_DIR")
	if baseline != "" {
		outputs, _ := os.ReadDir(out)
		for _, entry := range outputs {
			got, _ := os.ReadFile(filepath.Join(out, entry.Name()))
			want, e := os.ReadFile(filepath.Join(baseline, entry.Name()))
			if e != nil || sha256.Sum256(normalizeAudio(got)) != sha256.Sum256(normalizeAudio(want)) {
				t.Fatalf("output mismatch: %s", entry.Name())
			}
		}
	}
}

// ID3 libraries may reorder frames while iterating Go maps. Compare all frames
// and the exact audio payload, independent of frame order.
func normalizeAudio(data []byte) []byte {
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return data
	}
	size := 0
	for _, b := range data[6:10] {
		size = (size << 7) | int(b&127)
	}
	end := size + 10
	if end > len(data) {
		return data
	}
	frames := []string{}
	for pos := 10; pos+10 <= end && data[pos] != 0; {
		n := 0
		for _, b := range data[pos+4 : pos+8] {
			if data[3] == 4 {
				n = (n << 7) | int(b&127)
			} else {
				n = (n << 8) | int(b)
			}
		}
		next := pos + 10 + n
		if next > end {
			return data
		}
		frames = append(frames, string(data[pos:next]))
		pos = next
	}
	sort.Strings(frames)
	var out bytes.Buffer
	for _, f := range frames {
		out.WriteString(f)
	}
	out.Write(data[end:])
	return out.Bytes()
}
