package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Settings  Settings   `yaml:"settings"`
	Servers   []Server   `yaml:"servers"`
	Databases []Database `yaml:"databases"`
}

type Settings struct {
	DataDir        string    `yaml:"data_dir"`
	RequireMount   string    `yaml:"require_mount"`
	LogDir         string    `yaml:"log_dir"`
	RcloneRemote   string    `yaml:"rclone_remote"`
	RcloneBasePath string    `yaml:"rclone_base_path"`
	Retention      Retention `yaml:"retention"`
	Concurrency    int       `yaml:"concurrency"`
	RsyncExtra     []string  `yaml:"rsync_extra"`
}

type Retention struct {
	Daily  int `yaml:"daily"`
	Weekly int `yaml:"weekly"`
}

type Server struct {
	Name      string     `yaml:"name"`
	Enabled   bool       `yaml:"enabled"`
	Host      string     `yaml:"host"`
	User      string     `yaml:"user"`
	Port      int        `yaml:"port"`
	Auth      Auth       `yaml:"auth"`
	Paths     []string   `yaml:"paths"`
	Exclude   []string   `yaml:"exclude"`
	Databases []Database `yaml:"databases"`
}

type Auth struct {
	Type     string `yaml:"type"`
	KeyPath  string `yaml:"key_path"`
	Password string `yaml:"password"`
}

type Database struct {
	Name      string `yaml:"name"`
	Enabled   bool   `yaml:"enabled"`
	Engine    string `yaml:"engine"`
	Container string `yaml:"container"`
	DB        string `yaml:"db"`
	User      string `yaml:"user"`
	Password  string `yaml:"password"`
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	SSLMode   string `yaml:"sslmode"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	s := &c.Settings
	s.DataDir = expand(orDefault(s.DataDir, "~/svbackup/data"))
	s.RequireMount = expand(s.RequireMount)
	s.LogDir = expand(orDefault(s.LogDir, "~/svbackup/logs"))
	s.RcloneRemote = orDefault(s.RcloneRemote, "gdrive")
	s.RcloneBasePath = orDefault(s.RcloneBasePath, "svbackup")
	if s.Retention.Daily == 0 {
		s.Retention.Daily = 7
	}
	if s.Retention.Weekly == 0 {
		s.Retention.Weekly = 4
	}
	if s.Concurrency < 1 {
		s.Concurrency = 2
	}
	for i := range c.Servers {
		sv := &c.Servers[i]
		if sv.Port == 0 {
			sv.Port = 22
		}
		sv.Auth.KeyPath = expand(sv.Auth.KeyPath)
		for j := range sv.Databases {
			db := &sv.Databases[j]
			if db.Engine == "" {
				db.Engine = "postgres"
			}
			if db.Host == "" {
				db.Host = "127.0.0.1"
			}
			if db.Port == 0 {
				db.Port = 5432
			}
		}
	}
	for i := range c.Databases {
		db := &c.Databases[i]
		if db.Engine == "" {
			db.Engine = "postgres"
		}
		if db.Port == 0 {
			db.Port = 5432
		}
		if db.SSLMode == "" {
			db.SSLMode = "require"
		}
	}
}

func (c *Config) validate() error {
	seen := map[string]bool{}
	for _, sv := range c.Servers {
		if sv.Name == "" {
			return fmt.Errorf("server with empty name")
		}
		if seen[sv.Name] {
			return fmt.Errorf("duplicate server name %q", sv.Name)
		}
		seen[sv.Name] = true
		if sv.Host == "" || sv.User == "" {
			return fmt.Errorf("server %q: host and user required", sv.Name)
		}
		switch sv.Auth.Type {
		case "key":
			if sv.Auth.KeyPath == "" {
				return fmt.Errorf("server %q: auth.key_path required for type key", sv.Name)
			}
		case "password":
			if sv.Auth.Password == "" {
				return fmt.Errorf("server %q: auth.password required for type password", sv.Name)
			}
		default:
			return fmt.Errorf("server %q: auth.type must be key or password", sv.Name)
		}
		if len(sv.Paths) == 0 && len(sv.Databases) == 0 {
			return fmt.Errorf("server %q: nothing to back up (no paths, no databases)", sv.Name)
		}
		for _, db := range sv.Databases {
			if db.Engine != "postgres" {
				return fmt.Errorf("server %q db %q: only postgres engine supported", sv.Name, db.Name)
			}
			if db.Name == "" || db.DB == "" || db.User == "" {
				return fmt.Errorf("server %q: database entry needs name, db, user", sv.Name)
			}
		}
	}
	for _, db := range c.Databases {
		if db.Name == "" {
			return fmt.Errorf("standalone database with empty name")
		}
		if seen[db.Name] {
			return fmt.Errorf("duplicate name %q (standalone db collides with a server or another db)", db.Name)
		}
		seen[db.Name] = true
		if db.Engine != "postgres" {
			return fmt.Errorf("standalone db %q: only postgres engine supported", db.Name)
		}
		if db.DB == "" || db.User == "" || db.Host == "" {
			return fmt.Errorf("standalone db %q: needs db, user, host", db.Name)
		}
	}
	return nil
}

func (c *Config) Enabled(only string) []Server {
	var out []Server
	for _, sv := range c.Servers {
		if only != "" {
			if sv.Name == only {
				out = append(out, sv)
			}
			continue
		}
		if sv.Enabled {
			out = append(out, sv)
		}
	}
	return out
}

func (c *Config) EnabledDatabases(only string) []Database {
	var out []Database
	for _, db := range c.Databases {
		if only != "" {
			if db.Name == only {
				out = append(out, db)
			}
			continue
		}
		if db.Enabled {
			out = append(out, db)
		}
	}
	return out
}

func IsMounted(path string) (bool, error) {
	var st, parent syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return false, err
	}
	if err := syscall.Stat(filepath.Dir(path), &parent); err != nil {
		return false, err
	}
	return st.Dev != parent.Dev, nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func expand(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
