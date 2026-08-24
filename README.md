# svbackup

Pulls files **and** databases from a configured list of remote servers down to
this PC and mirrors them to Google Drive. Runs nightly via a systemd user timer
with catch-up if the PC was off.

## What it does per server

1. `rsync` each configured remote path into a local mirror, then `tar.gz` a
   point-in-time snapshot.
2. `pg_dump -Fc` each configured PostgreSQL database (host-native or via
   `docker exec <container>`), streamed over SSH into the snapshot dir.
3. Prune local snapshots to **7 daily + 4 weekly**.
4. `rclone copy` the snapshot to `gdrive:svbackup/<server>/<ts>/` and prune the
   remote to the same retention.

Snapshots live at `~/svbackup/data/<server>/snapshots/<ts>/`:
```
files.tar.gz            # tar of the rsync mirror
db-<name>.dump          # pg_dump custom format (restore with pg_restore)
```

## One-time setup

```bash
# 1. Tools
sudo apt install sshpass                 # for password-auth servers
sudo -v ; curl https://rclone.org/install.sh | sudo bash

# 2. Google Drive remote named "gdrive"
rclone config                            # n) new -> name: gdrive -> drive -> OAuth

# 3. Build + install the timer
./install.sh
loginctl enable-linger "$USER"           # timer fires even when logged out
```

## Config

Edit `config.yaml` (gitignored — holds secrets). See `config.example.yaml` for
the full schema. Each server has `paths[]` (rsync) and `databases[]` (pg_dump).
A DB entry with `container:` is dumped via `docker exec`; without it, host-native.

## Usage

```bash
./svbackup --check                 # verify tools, rclone remote, pg_dump reachability
./svbackup --probe                 # ssh each server, show path + DB sizes
./svbackup --server federal-ovh --once   # one server, full pipeline
./svbackup --once                  # all enabled servers
```

## Restore

```bash
# Files
tar -xzf files.tar.gz -C /some/restore/dir

# Database (custom format)
pg_restore -l db-calagopus-panel.dump          # inspect
pg_restore -d panel -U panel db-calagopus-panel.dump
```
