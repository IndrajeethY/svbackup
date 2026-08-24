package backup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"svbackup/internal/config"
)

type Result struct {
	Server     string
	Snapshot   string
	SnapshotTS string
	FilesBytes int64
	DBResults  []DBResult
	Errors     []error
	Duration   time.Duration
}

func (r Result) OK() bool { return len(r.Errors) == 0 }

func Run(sv config.Server, dataDir string, rsyncExtra []string, logf func(string, ...any)) Result {
	start := time.Now()
	ts := time.Now().Format("2006-01-02_1504")
	serverDir := filepath.Join(dataDir, sv.Name)
	mirrorDir := filepath.Join(serverDir, "mirror")
	snapDir := filepath.Join(serverDir, "snapshots", ts)

	res := Result{Server: sv.Name, Snapshot: snapDir, SnapshotTS: ts}

	if err := os.MkdirAll(snapDir, 0o700); err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("mkdir snapshot: %w", err))
		res.Duration = time.Since(start)
		return res
	}

	if len(sv.Paths) > 0 {
		if err := rsyncPaths(sv, mirrorDir, rsyncExtra, logf); err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("rsync: %w", err))
		} else {
			tarball := filepath.Join(snapDir, "files.tar.gz")
			if err := tarDir(mirrorDir, tarball); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("tar: %w", err))
			} else if fi, err := os.Stat(tarball); err == nil {
				res.FilesBytes = fi.Size()
			}
		}
	}

	for _, db := range sv.Databases {
		dbRes := dumpPostgres(sv, db, snapDir, logf)
		res.DBResults = append(res.DBResults, dbRes)
		if dbRes.Err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("db %s: %w", db.Name, dbRes.Err))
		}
	}

	res.Duration = time.Since(start)
	return res
}

func RunDatabase(db config.Database, dataDir string, logf func(string, ...any)) Result {
	start := time.Now()
	ts := time.Now().Format("2006-01-02_1504")
	snapDir := filepath.Join(dataDir, db.Name, "snapshots", ts)

	res := Result{Server: db.Name, Snapshot: snapDir, SnapshotTS: ts}

	if err := os.MkdirAll(snapDir, 0o700); err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("mkdir snapshot: %w", err))
		res.Duration = time.Since(start)
		return res
	}

	dbRes := dumpPostgresLocal(db, snapDir, logf)
	res.DBResults = append(res.DBResults, dbRes)
	if dbRes.Err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("db %s: %w", db.Name, dbRes.Err))
	}

	res.Duration = time.Since(start)
	return res
}

func rsyncPaths(sv config.Server, mirrorDir string, extra []string, logf func(string, ...any)) error {
	for _, p := range sv.Paths {
		dest := filepath.Join(mirrorDir, p)
		if err := os.MkdirAll(dest, 0o700); err != nil {
			return err
		}
		args := []string{"-a", "--delete"}
		args = append(args, extra...)
		for _, ex := range sv.Exclude {
			args = append(args, "--exclude", ex)
		}
		args = append(args, "-e", sshRemoteString(sv))
		src := fmt.Sprintf("%s@%s:%s/", sv.User, sv.Host, strings.TrimRight(p, "/"))
		args = append(args, src, dest+"/")

		logf("[%s] rsync %s", sv.Name, p)
		cmd := exec.Command("rsync", args...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("path %s: %w", p, err)
		}
	}
	return nil
}

func tarDir(srcDir, outPath string) error {
	if _, err := os.Stat(srcDir); err != nil {
		return fmt.Errorf("mirror missing: %w", err)
	}
	cmd := exec.Command("tar", "-czf", outPath, "-C", srcDir, ".")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
