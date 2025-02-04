package database

import "github.com/lucasberto/database-backup-tool/internal/ssh"

type Backup interface {
	Dump(sshClient *ssh.Client, dbName, user, password string) ([]byte, error)
}
