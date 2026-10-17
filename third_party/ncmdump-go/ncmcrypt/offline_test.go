package ncmcrypt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestMetadataNeverFetchesCover(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&calls, 1) }))
	defer s.Close()
	for _, flag := range []bool{false, true} {
		p := filepath.Join(t.TempDir(), "test.mp3")
		os.WriteFile(p, []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 0}, 0600)
		n := &NeteaseCloudMusic{Context: context.Background(), mFormat: Mp3, mDumpFilePath: p, mAlbumPicUrl: s.URL, mMetadata: &NeteaseClousMusicMetadata{mName: "Test", mArtist: "Artist"}}
		if _, e := n.FixMetadata(flag); e != nil {
			t.Fatal(e)
		}
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("unexpected network request")
	}
}

func TestDumpAcrossBufferBoundary(t *testing.T) {
	for _, header := range []string{"ID3", "fLaC"} {
		dir := t.TempDir()
		source := filepath.Join(dir, "source.ncm")
		plain := make([]byte, 600003)
		for i := range plain {
			plain[i] = byte(i * 7)
		}
		copy(plain, header)
		encrypted := append([]byte(nil), plain...)
		n := &NeteaseCloudMusic{Context: context.Background(), mFilePath: source}
		for i := range n.mKeyBox {
			n.mKeyBox[i] = byte(i)
		}
		for i := range encrypted {
			j := (i + 1) & 255
			encrypted[i] ^= n.mKeyBox[(n.mKeyBox[j]+n.mKeyBox[(int(n.mKeyBox[j])+j)&255])&255]
		}
		os.WriteFile(source, encrypted, 0600)
		n.mFileStream, _ = os.Open(source)
		defer n.Close()
		out := filepath.Join(dir, "out")
		os.Mkdir(out, 0700)
		if _, e := n.Dump(out); e != nil {
			t.Fatal(e)
		}
		got, _ := os.ReadFile(n.GetDumpFilePath())
		if string(got) != string(plain) {
			t.Fatal("audio mismatch across buffer boundary")
		}
	}
}
