package remote

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"svbackup/internal/retention"
)

type Remote struct {
	RemoteName string
	BasePath   string
}

func (r Remote) dest(sub string) string {
	return fmt.Sprintf("%s:%s/%s", r.RemoteName, r.BasePath, sub)
}

func (r Remote) Upload(localSnapDir, server, ts string, logf func(string, ...any)) error {
	target := r.dest(fmt.Sprintf("%s/%s", server, ts))
	logf("[%s] rclone copy -> %s", server, target)
	cmd := exec.Command("rclone", "copy", localSnapDir, target, "--transfers", "4")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type lsjsonEntry struct {
	Name  string `json:"Name"`
	IsDir bool   `json:"IsDir"`
}

func (r Remote) Prune(server string, daily, weekly int, logf func(string, ...any)) ([]string, error) {
	base := r.dest(server)
	out, err := exec.Command("rclone", "lsjson", base).Output()
	if err != nil {
		return nil, nil
	}
	var entries []lsjsonEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse lsjson: %w", err)
	}
	var snaps []retention.Snapshot
	for _, e := range entries {
		if !e.IsDir {
			continue
		}
		if s, ok := retention.Parse(e.Name); ok {
			snaps = append(snaps, s)
		}
	}
	_, del := retention.Plan(snaps, daily, weekly)
	var deleted []string
	for _, s := range del {
		target := r.dest(fmt.Sprintf("%s/%s", server, s.Name))
		logf("[%s] rclone purge %s", server, s.Name)
		cmd := exec.Command("rclone", "purge", target)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return deleted, fmt.Errorf("purge %s: %w", s.Name, err)
		}
		deleted = append(deleted, s.Name)
	}
	return deleted, nil
}

func Available(remoteName string) error {
	if _, err := exec.LookPath("rclone"); err != nil {
		return fmt.Errorf("rclone not found in PATH")
	}
	out, err := exec.Command("rclone", "listremotes").Output()
	if err != nil {
		return fmt.Errorf("rclone listremotes failed: %w", err)
	}
	want := remoteName + ":"
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == want {
			return nil
		}
	}
	return fmt.Errorf("rclone remote %q not configured (run: rclone config)", remoteName)
}
