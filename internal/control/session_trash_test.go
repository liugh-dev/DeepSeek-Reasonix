package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeSession plants a .jsonl and its .meta sidecar in dir, plus empty
// .ckpt/ and .jobs/ directories, so TrashSession has the full set of artifacts
// to move. It returns the absolute path of the .jsonl.
func writeFakeSession(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(dir, name+".jsonl")
	if err := os.WriteFile(jsonlPath, []byte("{\"role\":\"system\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonlPath+".meta", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ckptDir := strings.TrimSuffix(jsonlPath, ".jsonl") + ".ckpt"
	if err := os.MkdirAll(ckptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ckptDir, "turn-0.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Plant a job-artifact sidecar dir so the trash move of .jobs is covered.
	jobsDir := strings.TrimSuffix(jsonlPath, ".jsonl") + ".jobs"
	if err := os.MkdirAll(jobsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobsDir, "job-0.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return jsonlPath
}

func TestTrashSessionMovesArtifactsToTrashDir(t *testing.T) {
	dir := t.TempDir()
	path := writeFakeSession(t, dir, "session-1")

	if err := TrashSession(dir, path); err != nil {
		t.Fatal(err)
	}

	// Source files are gone from the live session dir.
	for _, p := range []string{path, path + ".meta", strings.TrimSuffix(path, ".jsonl") + ".ckpt", strings.TrimSuffix(path, ".jsonl") + ".jobs"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", p, err)
		}
	}

	// Moved files live under .trash/<key>/.
	itemDir := filepath.Join(dir, ".trash", filepath.Base(path))
	if st, err := os.Stat(itemDir); err != nil || !st.IsDir() {
		t.Fatalf("trash item dir missing: err=%v", err)
	}
	for _, want := range []string{filepath.Base(path), filepath.Base(path) + ".meta"} {
		if _, err := os.Stat(filepath.Join(itemDir, want)); err != nil {
			t.Fatalf("trash missing %s: %v", want, err)
		}
	}
	if st, err := os.Stat(filepath.Join(itemDir, "session-1.ckpt")); err != nil || !st.IsDir() {
		t.Fatalf("trash ckpt dir missing: err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(itemDir, "session-1.jobs")); err != nil || !st.IsDir() {
		t.Fatalf("trash jobs dir missing: err=%v", err)
	}

	// Trash meta file is present and well-formed.
	metaBytes, err := os.ReadFile(filepath.Join(itemDir, ".trash-meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta trashedSessionMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("decode trash meta: %v", err)
	}
	if meta.Key != filepath.Base(path) {
		t.Fatalf("trash meta key = %q, want %q", meta.Key, filepath.Base(path))
	}
	if meta.DeletedAt == 0 {
		t.Fatal("trash meta deletedAt should be set")
	}
}

func TestTrashSessionRejectsPathOutsideDir(t *testing.T) {
	dir := t.TempDir()
	outside := writeFakeSession(t, t.TempDir(), "escape")

	if err := TrashSession(dir, outside); err == nil {
		t.Fatal("TrashSession should refuse a path outside the session dir")
	} else if !strings.Contains(err.Error(), "outside") {
		t.Fatalf("error should mention escape, got %v", err)
	}
}

func TestTrashSessionRejectsNonJSONL(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, "session.meta")
	if err := os.WriteFile(meta, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := TrashSession(dir, meta); err == nil {
		t.Fatal("TrashSession should refuse a non-.jsonl file")
	}
}

func TestTrashSessionRejectsEmptyDir(t *testing.T) {
	if err := TrashSession("", "/tmp/whatever.jsonl"); err == nil {
		t.Fatal("TrashSession should refuse an empty session dir")
	}
}

func TestTrashSessionIdempotentWhenMissing(t *testing.T) {
	dir := t.TempDir()
	ghost := filepath.Join(dir, "ghost.jsonl")
	// No file exists; TrashSession should treat it as a successful no-op.
	if err := TrashSession(dir, ghost); err != nil {
		t.Fatalf("TrashSession on missing file should be a no-op, got %v", err)
	}
}

func TestTrashSessionRejectsAlreadyTrashed(t *testing.T) {
	dir := t.TempDir()
	path := writeFakeSession(t, dir, "dup")
	if err := TrashSession(dir, path); err != nil {
		t.Fatal(err)
	}
	// Plant another file under the same basename and try again — should
	// fail with the ErrSessionAlreadyInTrash sentinel so callers can branch.
	path2 := writeFakeSession(t, dir, "dup")
	err := TrashSession(dir, path2)
	if err == nil {
		t.Fatal("TrashSession should refuse when the trash entry already exists")
	}
	if !strings.Contains(err.Error(), "trash") {
		t.Fatalf("error should mention trash, got %v", err)
	}
}

func TestTrashSessionRefusesSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	external := writeFakeSession(t, outside, "ext")
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}
	err := TrashSession(dir, link)
	if err == nil {
		t.Fatal("TrashSession should refuse a symlink escaping the session dir")
	}
}

func TestControllerDeleteSessionRefusesActive(t *testing.T) {
	dir := t.TempDir()
	active := writeFakeSession(t, dir, "active")
	c := New(Options{SessionDir: dir, SessionPath: active})

	err := c.DeleteSession(active)
	if err == nil {
		t.Fatal("DeleteSession should refuse the active session")
	}
	// The file is still there.
	if _, statErr := os.Stat(active); statErr != nil {
		t.Fatalf("active session file should still exist, stat err=%v", statErr)
	}
}

func TestControllerDeleteSessionMovesInactive(t *testing.T) {
	dir := t.TempDir()
	active := writeFakeSession(t, dir, "active")
	other := writeFakeSession(t, dir, "other")
	c := New(Options{SessionDir: dir, SessionPath: active})

	if err := c.DeleteSession(other); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("other session should be moved, stat err=%v", err)
	}
	itemDir := filepath.Join(dir, ".trash", "other.jsonl")
	if _, err := os.Stat(itemDir); err != nil {
		t.Fatalf("trash entry missing: %v", err)
	}
	// Active untouched.
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active session should be intact, stat err=%v", err)
	}
}

func TestControllerDeleteSessionRefusesUnconfiguredDir(t *testing.T) {
	c := New(Options{}) // no SessionDir
	err := c.DeleteSession("/tmp/whatever.jsonl")
	if err == nil {
		t.Fatal("DeleteSession should fail when no session dir is configured")
	}
}
