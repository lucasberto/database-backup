package mysql

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
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

// NewRemoteID gera um identificador único usado nos nomes dos arquivos
// temporários remotos (/tmp/mydump-<id>.cnf, dumps intermediários).
// Servidores lógicos distintos podem apontar para a mesma máquina física,
// e execuções simultâneas da ferramenta compartilham o mesmo /tmp remoto —
// sem o id, uma execução apaga os arquivos da outra.
func NewRemoteID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func remoteConfigPath(remoteID string) string {
	return fmt.Sprintf("/tmp/mydump-%s.cnf", remoteID)
}

func (m *MySQL) CreateConfigFile(sshClient *ssh.Client, user, password string, port int, remoteID string) error {
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

	configPath := remoteConfigPath(remoteID)
	setupCmd := fmt.Sprintf("rm -f %s && cat > %s << 'EOL'\n%s\nEOL\nchmod 600 %s", configPath, configPath, tmpConfig, configPath)
	return session.Run(setupCmd)
}

func (m *MySQL) CleanupConfigFile(sshClient *ssh.Client, remoteID string) error {
	session, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	return session.Run(fmt.Sprintf("rm -f %s", remoteConfigPath(remoteID)))
}

func (m *MySQL) Dump(sshClient *ssh.Client, dbName string, filePath string, remoteID string, progress *mpb.Progress) error {

	displayName := dbName
	if len(dbName) > 40 {
		displayName = dbName[:37] + "..."
	}

	// Server-side dump and compression
	tmpSqlFile := fmt.Sprintf("/tmp/%s_dump-%s.sql", dbName, remoteID)
	tmpGzFile := fmt.Sprintf("/tmp/%s_dump-%s.sql.gz", dbName, remoteID)

	// Step 1: Create dump file on server
	dumpBar := progress.New(0,
		mpb.BarStyle(),
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Dumping %s... ", displayName), decor.WC{W: 45, C: decor.DindentRight}),
		),
	)

	dumpSession, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		dumpBar.Abort(false)
		return fmt.Errorf("failed to create dump session: %v", err)
	}
	defer dumpSession.Close()

	var dumpStderr bytes.Buffer
	dumpSession.Stderr = &dumpStderr

	dumpCmd := fmt.Sprintf("mysqldump --defaults-file=%s --quick --lock-tables=false --skip-routines --skip-triggers --skip-events %s > %s", remoteConfigPath(remoteID), dbName, tmpSqlFile)

	err = dumpSession.Run(dumpCmd)
	dumpBar.SetTotal(1, true)
	dumpBar.Increment()

	if err != nil {
		return fmt.Errorf("mysqldump failed: %v: %s", err, dumpStderr.String())
	}

	// Step 2: Compress file on server
	compressBar := progress.New(0,
		mpb.BarStyle(),
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Compressing %s... ", displayName), decor.WC{W: 45, C: decor.DindentRight}),
		),
	)

	compressSession, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		compressBar.Abort(false)
		return fmt.Errorf("failed to create compress session: %v", err)
	}
	defer compressSession.Close()

	var compressStderr bytes.Buffer
	compressSession.Stderr = &compressStderr

	compressCmd := fmt.Sprintf("gzip -1 -c %s > %s && rm %s", tmpSqlFile, tmpGzFile, tmpSqlFile)

	err = compressSession.Run(compressCmd)
	compressBar.SetTotal(1, true)
	compressBar.Increment()

	if err != nil {
		return fmt.Errorf("compression failed: %v: %s", err, compressStderr.String())
	}

	// Step 3: Get compressed file size for accurate progress
	sizeSession, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		return fmt.Errorf("failed to create size session: %v", err)
	}
	defer sizeSession.Close()

	var sizeStdout bytes.Buffer
	var sizeStderr bytes.Buffer
	sizeSession.Stdout = &sizeStdout
	sizeSession.Stderr = &sizeStderr

	sizeCmd := fmt.Sprintf("stat -c%%s %s", tmpGzFile)

	err = sizeSession.Run(sizeCmd)
	if err != nil {
		return fmt.Errorf("failed to get file size: %v: %s", err, sizeStderr.String())
	}

	var fileSize int64
	if _, err := fmt.Sscanf(strings.TrimSpace(sizeStdout.String()), "%d", &fileSize); err != nil {
		return fmt.Errorf("failed to parse file size: %v", err)
	}

	// Step 4: Download compressed file
	downloadBar := progress.New(fileSize,
		mpb.BarStyle(),
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Downloading %s ", displayName), decor.WC{W: 45, C: decor.DindentRight}),
		),
		mpb.AppendDecorators(
			decor.CountersKibiByte("%.2f / %.2f"),
			decor.NewPercentage(" (%d)"),
		),
	)

	downloadSession, err := sshClient.GetSSHClient().NewSession()
	if err != nil {
		downloadBar.Abort(false)
		return fmt.Errorf("failed to create download session: %v", err)
	}
	defer downloadSession.Close()

	cpw, err := NewCompressedProgressWriter(downloadBar, filePath)
	if err != nil {
		downloadBar.Abort(false)
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer cpw.Close()

	var downloadStderr bytes.Buffer
	downloadSession.Stdout = cpw
	downloadSession.Stderr = &downloadStderr

	downloadCmd := fmt.Sprintf("cat %s && rm %s", tmpGzFile, tmpGzFile)

	err = downloadSession.Run(downloadCmd)
	if err != nil {
		return fmt.Errorf("download failed: %v: %s", err, downloadStderr.String())
	}

	return nil
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
