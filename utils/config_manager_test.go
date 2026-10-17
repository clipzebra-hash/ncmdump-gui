package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyConfigDefaultsToKeepSource(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"save_to":"custom","path":"/test"}`), 0600)
	cm := &ConfigManager{FilePath: p}
	pref := cm.Load()
	if pref == nil || pref.DeleteSource {
		t.Fatal("legacy config enabled deletion")
	}
	pref.DeleteSource = true
	if !cm.Save(pref) {
		t.Fatal("save failed")
	}
	loaded := cm.Load()
	if loaded == nil || !loaded.DeleteSource || loaded.Path != "/test" {
		t.Fatal(loaded)
	}
}
