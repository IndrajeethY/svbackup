package backup

import (
	"fmt"
	"os/exec"
	"strings"

	"svbackup/internal/config"
)

func PgDumpVersionCmd(sv config.Server, db config.Database) *exec.Cmd {
	var remote string
	if db.Container != "" {
		remote = fmt.Sprintf("docker exec %s pg_dump --version", shQuote(db.Container))
	} else {
		remote = "pg_dump --version"
	}
	argv := sshExec(sv, remote)
	return exec.Command(argv[0], argv[1:]...)
}

func PgDumpLocalCheckCmd(db config.Database) *exec.Cmd {
	return exec.Command("psql", "-d", pgDSN(db), "-tAc", "SELECT 1;")
}

func Probe(sv config.Server) []string {
	var lines []string

	for _, p := range sv.Paths {
		remote := fmt.Sprintf("du -sh %s 2>/dev/null | cut -f1; test -e %s || echo '(path not found)'", shQuote(p), shQuote(p))
		out := strings.TrimSpace(runRemote(sv, remote))
		lines = append(lines, fmt.Sprintf("path %s = %s", p, out))
	}

	for _, db := range sv.Databases {
		sql := fmt.Sprintf("SELECT pg_size_pretty(pg_database_size('%s'));", db.DB)
		var remote string
		if db.Container != "" {
			remote = fmt.Sprintf("docker exec -e PGPASSWORD=%s %s psql -U %s -tAc %s %s",
				shQuote(db.Password), shQuote(db.Container), shQuote(db.User), shQuote(sql), shQuote(db.DB))
		} else {
			remote = fmt.Sprintf("PGPASSWORD=%s psql -h %s -p %d -U %s -tAc %s %s",
				shQuote(db.Password), shQuote(db.Host), db.Port, shQuote(db.User), shQuote(sql), shQuote(db.DB))
		}
		out := strings.TrimSpace(runRemote(sv, remote))
		if out == "" {
			out = "(query failed)"
		}
		lines = append(lines, fmt.Sprintf("db %s (%s) = %s", db.Name, db.DB, out))
	}

	if len(lines) == 0 {
		lines = append(lines, "(nothing configured)")
	}
	return lines
}

func runRemote(sv config.Server, remoteCmd string) string {
	argv := sshExec(sv, remoteCmd)
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("ERROR: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out)
}
