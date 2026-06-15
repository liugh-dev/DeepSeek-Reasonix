package control

// Session trash: move a saved session's transcript, sidecars, checkpoints, and
// subagent artifacts out of the active session dir into a local ".trash/<key>/"
// subdirectory. The same shape lives in the desktop history panel (see
// desktop/sessions.go) — both frontends delete by moving into trash so the
// operation is reversible. Trash files are siblings of the live session dir
// (not the system trash), so a `reasonix run` from another shell can't see
// them via agent.ListSessions and they survive across reinstalls of the app.
//
// The trash helpers were originally in `desktop/sessions.go`; they were lifted
// here so the chat TUI can use the same flow without depending on the desktop
// binary. Restore + purge stay desktop-only (the chat TUI has no UI for them
// yet) and continue to read the .trash-meta.json file this package writes.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/jobs"
)

// ErrActiveSession is returned when a delete targets the session currently
// in use. Mirrors the desktop sentinel so callers can errors.Is on it.
var ErrActiveSession = errors.New("can't delete the session you're in — start a new one first")

// ErrSessionAlreadyInTrash is returned when a delete targets a key that
// already has an entry in the local trash. Restoring or purging the existing
// entry should resolve it.
var ErrSessionAlreadyInTrash = errors.New("session already exists in trash")

const sessionTrashDir = ".trash"
const sessionTrashMetaFile = ".trash-meta.json"

// trashedSessionMeta is the sidecar written into each trash item. Key is the
// basename of the session file (e.g. "2024-05-12T10-15-22-abc.jsonl"); the
// DeletedAt millisecond timestamp is what the desktop History panel sorts by.
type trashedSessionMeta struct {
	Key       string `json:"key"`
	DeletedAt int64  `json:"deletedAt"`
}

// TrashSession moves sessionPath (and its .meta sidecar, .ckpt directory, .jobs
// artifact directory, and subagent artifacts) into <sessionDir>/.trash/<key>/.
// The path is validated to be a .jsonl file inside sessionDir. Existing trash
// entries with the same key are NOT overwritten; the caller should restore or
// purge the existing entry first.
func TrashSession(sessionDir, sessionPath string) error {
	sessionDir = strings.TrimSpace(sessionDir)
	if sessionDir == "" {
		return fmt.Errorf("empty session dir")
	}
	abs, key, err := validateSessionPathForTrash(sessionDir, sessionPath)
	if err != nil {
		return err
	}
	return trashSessionArtifacts(sessionDir, abs, key)
}

func trashSessionArtifacts(sessionDir, sessionPath, key string) error {
	// Missing session is a no-op (matches desktop: a second delete on the
	// same path is idempotent, so retries don't surface as errors).
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	itemDir := filepath.Join(sessionDir, sessionTrashDir, key)
	if _, err := os.Stat(itemDir); err == nil {
		return fmt.Errorf("%w: %s", ErrSessionAlreadyInTrash, key)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		return err
	}

	if err := movePathIfExists(sessionPath, filepath.Join(itemDir, key)); err != nil {
		return err
	}
	if err := movePathIfExists(sessionPath+".meta", filepath.Join(itemDir, key+".meta")); err != nil {
		return err
	}
	// Checkpoints live next to the .jsonl as "<base>.ckpt/" — the base is the
	// filename without the extension, matching how the controller computes it.
	ckptName := strings.TrimSuffix(key, filepath.Ext(key)) + ".ckpt"
	ckptSrc := strings.TrimSuffix(sessionPath, filepath.Ext(sessionPath)) + ".ckpt"
	if err := movePathIfExists(ckptSrc, filepath.Join(itemDir, ckptName)); err != nil {
		return err
	}
	// Background-job artifacts live next to the .jsonl as "<base>.jobs/" (see
	// jobs.ArtifactDir). Move them too so a deleted session's job sidecars
	// don't linger in the live session dir — this matches the desktop trash
	// flow the helpers were lifted from.
	jobsName := strings.TrimSuffix(key, filepath.Ext(key)) + ".jobs"
	jobsSrc := jobs.ArtifactDir(sessionPath)
	if err := movePathIfExists(jobsSrc, filepath.Join(itemDir, jobsName)); err != nil {
		return err
	}
	if err := trashSubagentArtifacts(sessionDir, sessionPath, itemDir); err != nil {
		return err
	}

	meta := trashedSessionMeta{Key: key, DeletedAt: time.Now().UnixMilli()}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(itemDir, sessionTrashMetaFile), b, 0o644); err != nil {
		return err
	}
	return nil
}

func trashSubagentArtifacts(sessionDir, sessionPath, itemDir string) error {
	artifacts, err := agent.ListSubagentsByParent(sessionDir, agent.BranchID(sessionPath))
	if err != nil {
		return err
	}
	if len(artifacts) == 0 {
		return nil
	}
	trashSubagentDir := filepath.Join(itemDir, "subagents")
	for _, artifact := range artifacts {
		if err := movePathIfExists(artifact.SessionPath, filepath.Join(trashSubagentDir, filepath.Base(artifact.SessionPath))); err != nil {
			return err
		}
		if err := movePathIfExists(artifact.MetaPath, filepath.Join(trashSubagentDir, filepath.Base(artifact.MetaPath))); err != nil {
			return err
		}
	}
	return nil
}

func movePathIfExists(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// validateSessionPathForTrash returns an absolute path inside sessionDir for
// sessionPath, plus the session's basename. The session must be a .jsonl file
// and must not escape the session dir via symlinks. The desktop package has a
// near-identical helper; we duplicate it here to avoid a cross-package import
// from control → desktop (the desktop package imports control, not the other
// way around).
func validateSessionPathForTrash(sessionDir, sessionPath string) (string, string, error) {
	if strings.TrimSpace(sessionPath) == "" {
		return "", "", fmt.Errorf("empty session path")
	}
	absDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return "", "", err
	}
	path := sessionPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(absDir, path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	if filepath.Ext(absPath) != ".jsonl" {
		return "", "", fmt.Errorf("not a session file: %s", sessionPath)
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("session path outside session dir: %s", sessionPath)
	}
	// If the file exists, also follow the symlink target to make sure it
	// doesn't sneak out of the dir; missing files are accepted because the
	// caller may have just rotated the path. The move itself will surface a
	// clear "no such file" error in that case.
	if info, err := os.Lstat(absPath); err == nil {
		if info.IsDir() {
			return "", "", fmt.Errorf("not a session file: %s", sessionPath)
		}
		realDir, dirErr := filepath.EvalSymlinks(absDir)
		if dirErr != nil {
			realDir = absDir
		}
		realPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return "", "", err
		}
		rel, err := filepath.Rel(realDir, realPath)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
			return "", "", fmt.Errorf("session path escapes session dir: %s", sessionPath)
		}
	} else if !os.IsNotExist(err) {
		return "", "", err
	}
	return absPath, filepath.Base(absPath), nil
}
