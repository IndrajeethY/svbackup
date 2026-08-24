package backup

import (
	"fmt"
	"strings"

	"svbackup/internal/config"
)

func sshBase(sv config.Server) []string {
	var argv []string
	if sv.Auth.Type == "password" {
		argv = append(argv, "sshpass", "-p", sv.Auth.Password)
	}
	argv = append(argv, "ssh",
		"-p", fmt.Sprintf("%d", sv.Port),
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=no",
		"-o", "ConnectTimeout=15",
	)
	if sv.Auth.Type == "key" {
		argv = append(argv, "-i", sv.Auth.KeyPath)
	}
	return argv
}

func sshRemoteString(sv config.Server) string {
	return strings.Join(sshBase(sv), " ")
}

func sshExec(sv config.Server, remoteCmd string) []string {
	argv := sshBase(sv)
	argv = append(argv, fmt.Sprintf("%s@%s", sv.User, sv.Host), remoteCmd)
	return argv
}
