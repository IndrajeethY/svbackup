package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"svbackup/internal/backup"
	"svbackup/internal/config"
	"svbackup/internal/notify"
	"svbackup/internal/remote"
	"svbackup/internal/retention"
)

func main() {
	var (
		cfgPath = flag.String("config", defaultConfigPath(), "path to config.yaml")
		once    = flag.Bool("once", false, "run all enabled servers once")
		only    = flag.String("server", "", "only this server (implies once)")
		check   = flag.Bool("check", false, "pre-flight: verify tools + rclone remote + pg_dump reachability")
		probe   = flag.Bool("probe", false, "ssh each server and report path sizes + DB sizes")
	)
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(2)
	}

	switch {
	case *check:
		os.Exit(runCheck(cfg))
	case *probe:
		os.Exit(runProbe(cfg))
	case *once || *only != "":
		os.Exit(runOnce(cfg, *only))
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func defaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "svbackup", "config.yaml")
}

func runOnce(cfg *config.Config, only string) int {
	log, err := notify.New(cfg.Settings.LogDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "log init:", err)
		return 1
	}
	defer log.Close()

	if rm := cfg.Settings.RequireMount; rm != "" {
		mounted, err := config.IsMounted(rm)
		if err != nil {
			log.Printf("ABORT: require_mount %q not accessible: %v", rm, err)
			return 1
		}
		if !mounted {
			log.Printf("ABORT: require_mount %q is not a mounted filesystem — refusing to back up to unmounted target", rm)
			return 1
		}
	}

	servers := cfg.Enabled(only)
	databases := cfg.EnabledDatabases(only)
	if len(servers) == 0 && len(databases) == 0 {
		log.Printf("no servers or databases to run (only=%q)", only)
		return 1
	}

	rem := remote.Remote{RemoteName: cfg.Settings.RcloneRemote, BasePath: cfg.Settings.RcloneBasePath}
	rcloneOK := remote.Available(cfg.Settings.RcloneRemote) == nil
	if !rcloneOK {
		log.Printf("WARNING: rclone remote %q unavailable — local backups only, no upload", cfg.Settings.RcloneRemote)
	}

	sem := make(chan struct{}, cfg.Settings.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []backup.Result

	finish := func(name string, res backup.Result) {
		snapRoot := filepath.Join(cfg.Settings.DataDir, name, "snapshots")
		if del, err := retention.PruneLocal(snapRoot, cfg.Settings.Retention.Daily, cfg.Settings.Retention.Weekly); err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("local prune: %w", err))
		} else if len(del) > 0 {
			log.Printf("[%s] pruned %d local snapshot(s)", name, len(del))
		}

		if rcloneOK && res.OK() {
			if err := rem.Upload(res.Snapshot, name, res.SnapshotTS, log.Printf); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("upload: %w", err))
			} else if _, err := rem.Prune(name, cfg.Settings.Retention.Daily, cfg.Settings.Retention.Weekly, log.Printf); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("remote prune: %w", err))
			}
		}

		mu.Lock()
		results = append(results, res)
		mu.Unlock()
	}

	for _, sv := range servers {
		wg.Add(1)
		go func(sv config.Server) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := backup.Run(sv, cfg.Settings.DataDir, cfg.Settings.RsyncExtra, log.Printf)
			finish(sv.Name, res)
		}(sv)
	}

	for _, db := range databases {
		wg.Add(1)
		go func(db config.Database) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := backup.RunDatabase(db, cfg.Settings.DataDir, log.Printf)
			finish(db.Name, res)
		}(db)
	}
	wg.Wait()

	failed := 0
	log.Printf("==== summary ====")
	for _, r := range results {
		status := "OK"
		if !r.OK() {
			status = "FAIL"
			failed++
		}
		log.Printf("[%s] %s  files=%s  dbs=%d  took=%s",
			r.Server, status, notify.HumanBytes(r.FilesBytes), len(r.DBResults), r.Duration.Round(1e9))
		for _, e := range r.Errors {
			log.Printf("    error: %v", e)
		}
	}
	if failed > 0 {
		log.Printf("%d/%d servers failed", failed, len(results))
		return 1
	}
	log.Printf("all %d servers OK", len(results))
	return 0
}

func runCheck(cfg *config.Config) int {
	ok := true
	fmt.Println("== local tools ==")
	for _, tool := range []string{"rsync", "tar", "rclone", "ssh"} {
		if _, err := exec.LookPath(tool); err != nil {
			fmt.Printf("  MISSING %s\n", tool)
			ok = false
		} else {
			fmt.Printf("  ok      %s\n", tool)
		}
	}

	needSSHpass := false
	for _, sv := range cfg.Servers {
		if sv.Auth.Type == "password" {
			needSSHpass = true
		}
	}
	if needSSHpass {
		if _, err := exec.LookPath("sshpass"); err != nil {
			fmt.Println("  MISSING sshpass (needed for password-auth servers)")
			ok = false
		} else {
			fmt.Println("  ok      sshpass")
		}
	}

	fmt.Println("== rclone remote ==")
	if err := remote.Available(cfg.Settings.RcloneRemote); err != nil {
		fmt.Printf("  FAIL %v\n", err)
		ok = false
	} else {
		fmt.Printf("  ok   remote %q reachable\n", cfg.Settings.RcloneRemote)
	}

	fmt.Println("== database pg_dump reachability ==")
	for _, sv := range cfg.Enabled("") {
		for _, db := range sv.Databases {
			cmd := backup.PgDumpVersionCmd(sv, db)
			if err := cmd.Run(); err != nil {
				fmt.Printf("  FAIL [%s/%s] %v\n", sv.Name, db.Name, err)
				ok = false
			} else {
				fmt.Printf("  ok   [%s/%s] pg_dump reachable\n", sv.Name, db.Name)
			}
		}
	}
	if len(cfg.EnabledDatabases("")) > 0 {
		if _, err := exec.LookPath("pg_dump"); err != nil {
			fmt.Println("  MISSING pg_dump (needed for standalone databases) — install postgresql-client")
			ok = false
		}
		for _, db := range cfg.EnabledDatabases("") {
			cmd := backup.PgDumpLocalCheckCmd(db)
			if err := cmd.Run(); err != nil {
				fmt.Printf("  FAIL [db:%s] %v\n", db.Name, err)
				ok = false
			} else {
				fmt.Printf("  ok   [db:%s] local pg connection reachable\n", db.Name)
			}
		}
	}

	if ok {
		fmt.Println("\nAll checks passed.")
		return 0
	}
	fmt.Println("\nSome checks failed — see above.")
	return 1
}

func runProbe(cfg *config.Config) int {
	for _, sv := range cfg.Enabled("") {
		fmt.Printf("== %s (%s@%s) ==\n", sv.Name, sv.User, sv.Host)
		for _, out := range backup.Probe(sv) {
			fmt.Println("  " + out)
		}
	}
	return 0
}
