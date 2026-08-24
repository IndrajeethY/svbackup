package backup

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"svbackup/internal/config"
)

type DBResult struct {
	Name  string
	File  string
	Bytes int64
	Err   error
}

func dumpPostgres(sv config.Server, db config.Database, snapDir string, logf func(string, ...any)) DBResult {
	out := filepath.Join(snapDir, "db-"+db.Name+".dump")
	res := DBResult{Name: db.Name, File: out}

	remote := pgDumpRemoteCmd(db)
	logf("[%s] pg_dump %s (%s)", sv.Name, db.Name, db.DB)

	f, err := os.Create(out)
	if err != nil {
		res.Err = err
		return res
	}
	defer f.Close()

	argv := sshExec(sv, remote)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		res.Err = err
		return res
	}
	if fi, err := os.Stat(out); err == nil {
		res.Bytes = fi.Size()
		if fi.Size() == 0 {
			res.Err = fmt.Errorf("dump is empty")
		}
	}
	return res
}

func pgDumpRemoteCmd(db config.Database) string {
	if db.Container != "" {
		return fmt.Sprintf(
			"docker exec -e PGPASSWORD=%s %s pg_dump -U %s -Fc %s",
			shQuote(db.Password), shQuote(db.Container), shQuote(db.User), shQuote(db.DB),
		)
	}
	return fmt.Sprintf(
		"PGPASSWORD=%s pg_dump -h %s -p %d -U %s -Fc %s",
		shQuote(db.Password), shQuote(db.Host), db.Port, shQuote(db.User), shQuote(db.DB),
	)
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func pgDSN(db config.Database) string {
	u := url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(db.User, db.Password),
		Host:   fmt.Sprintf("%s:%d", db.Host, db.Port),
		Path:   "/" + db.DB,
	}
	q := url.Values{}
	if db.SSLMode != "" {
		q.Set("sslmode", db.SSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func dumpPostgresLocal(db config.Database, snapDir string, logf func(string, ...any)) DBResult {
	out := filepath.Join(snapDir, "db-"+db.Name+".dump")
	res := DBResult{Name: db.Name, File: out}

	logf("[db:%s] pg_dump %s@%s/%s (local pg_dump, no ssh)", db.Name, db.User, db.Host, db.DB)

	f, err := os.Create(out)
	if err != nil {
		res.Err = err
		return res
	}
	defer f.Close()

	cmd := exec.Command("pg_dump", "-Fc", "-d", pgDSN(db))
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		res.Err = err
		return res
	}
	if fi, err := os.Stat(out); err == nil {
		res.Bytes = fi.Size()
		if fi.Size() == 0 {
			res.Err = fmt.Errorf("dump is empty")
		}
	}
	return res
}
