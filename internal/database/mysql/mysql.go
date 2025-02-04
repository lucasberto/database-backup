package mysql

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/lucasberto/database-backup-tool/internal/ssh"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type MySQL struct{}

func New() *MySQL {
	return &MySQL{}
}

func (m *MySQL) Dump(sshClient *ssh.Client, dbName, user, password string, port int, progress *mpb.Progress) ([]byte, error) {
	session, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	bar := progress.AddBar(-1,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Dumping %s ", dbName), decor.WC{W: len(dbName) + 20, C: decor.DindentRight}),
			decor.CurrentKibiByte("%.2f"),
		),
	)

	// var stdout bytes.Buffer
	var stderr bytes.Buffer

	cpw := NewCompressedProgressWriter(bar)
	session.Stdout = cpw
	session.Stderr = &stderr

	cmd := fmt.Sprintf("mysqldump -h127.0.0.1 -P%d -u%s -p%s %s",
		port,
		user,
		password,
		dbName,
	)

	err = session.Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("mysqldump failed: %v: %s", err, stderr.String())
	}

	cpw.Close()

	return cpw.Bytes(), nil
}

func (m *MySQL) ListDatabases(sshClient *ssh.Client, user, password string, port int) ([]string, error) {
	session, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	cmd := fmt.Sprintf("mysql -h127.0.0.1 -P%d -u%s -p%s -N -e 'SHOW DATABASES' | grep -Ev '^(information_schema|performance_schema|mysql|sys)$'",
		port,
		user,
		password,
	)

	err = session.Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %v: %s", err, stderr.String())
	}

	databases := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	return databases, nil
}
