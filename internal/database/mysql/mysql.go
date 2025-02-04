package mysql

import (
	"bytes"
	"fmt"
	"strconv"
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

	sizeSession, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create size session: %v", err)
	}

	var sizeOutput bytes.Buffer
	sizeSession.Stdout = &sizeOutput

	sizeCmd := fmt.Sprintf(`mysql -N -h127.0.0.1 -P%d -u%s -p%s -e "
        SELECT SUM(data_length + index_length) 
        FROM information_schema.tables 
        WHERE table_schema = '%s'
        GROUP BY table_schema;"`, port, user, password, dbName)

	err = sizeSession.Run(sizeCmd)
	sizeSession.Close()

	var estimatedSize int64 = 100 * 1024 * 1024
	if err == nil {
		size, err := strconv.ParseInt(strings.TrimSpace(sizeOutput.String()), 10, 64)
		if err == nil {
			estimatedSize = size
		}
	}

	bar := progress.AddBar(-1,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Dumping %s ", dbName), decor.WC{W: len(dbName) + 20, C: decor.DindentRight}),
			decor.CurrentKibiByte("%.2f"),
		),
	)

	// var stdout bytes.Buffer
	var stderr bytes.Buffer

	cpw := NewCompressedProgressWriter(bar, estimatedSize)
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
