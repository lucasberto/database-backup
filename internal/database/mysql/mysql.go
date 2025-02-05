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

func (m *MySQL) CreateConfigFile(sshClient *ssh.Client, user, password string, port int) error {
	session, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	tmpConfig := fmt.Sprintf(`[client]
host=127.0.0.1
user=%s
password=%s
port=%d`, user, password, port)

	setupCmd := fmt.Sprintf("rm -f /tmp/mydump.cnf && cat > /tmp/mydump.cnf << 'EOL'\n%s\nEOL\nchmod 600 /tmp/mydump.cnf", tmpConfig)
	return session.Run(setupCmd)
}

func (m *MySQL) CleanupConfigFile(sshClient *ssh.Client) error {
	session, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	return session.Run("rm -f /tmp/mydump.cnf")
}

func (m *MySQL) Dump(sshClient *ssh.Client, dbName string, progress *mpb.Progress) ([]byte, error) {

	// create new session for the actual dump
	session, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	displayName := dbName
	if len(dbName) > 40 {
		displayName = dbName[:37] + "..."
	}

	bar := progress.New(-1,
		mpb.BarStyle(),
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Dumping %s ", displayName), decor.WC{W: 45, C: decor.DindentRight}),
			decor.CurrentKibiByte("%.2f"),
		),
	)

	var stderr bytes.Buffer

	cpw := NewCompressedProgressWriter(bar)
	session.Stdout = cpw
	session.Stderr = &stderr
	defer cpw.Close()

	cmd := fmt.Sprintf("mysqldump --defaults-file=/tmp/mydump.cnf %s", dbName)

	err = session.Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("mysqldump failed: %v: %s", err, stderr.String())
	}

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
