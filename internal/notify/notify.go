package notify

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Logger struct {
	*log.Logger
	f *os.File
}

func New(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, err
	}
	name := filepath.Join(logDir, "run-"+time.Now().Format("2006-01-02_1504")+".log")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	w := io.MultiWriter(os.Stdout, f)
	return &Logger{Logger: log.New(w, "", log.LstdFlags), f: f}, nil
}

func (l *Logger) Printf(format string, args ...any) {
	l.Logger.Printf(format, args...)
}

func (l *Logger) Close() {
	if l.f != nil {
		l.f.Close()
	}
}

func HumanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
