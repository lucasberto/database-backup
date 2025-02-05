package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lucasberto/database-backup-tool/internal/config"
	"github.com/lucasberto/database-backup-tool/internal/credentials"
	"github.com/lucasberto/database-backup-tool/internal/database/mysql"
	"github.com/lucasberto/database-backup-tool/internal/ssh"
	"github.com/vbauerster/mpb/v8"
)

type BackupResult struct {
	ServerName string
	Database   string
	Success    bool
	Error      error
	StartTime  time.Time
	EndTime    time.Time
	FileSize   int64
}

var globalConfig *config.Config

func ensureOutputDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func cleanupOldBackups(serverDir string, retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

	entries, err := os.ReadDir(serverDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql.gz") {
			info, err := entry.Info()
			if err != nil {
				continue
			}

			if info.ModTime().Before(cutoffTime) {
				fullPath := filepath.Join(serverDir, entry.Name())
				if err := os.Remove(fullPath); err != nil {
					return fmt.Errorf("failed to remove old backup %s: %v", fullPath, err)
				}
			}
		}
	}
	return nil
}

func backupDatabase(client *ssh.Client, serverName, serverDir, dbName string, mysqlBackup *mysql.MySQL, progress *mpb.Progress, resultsChan chan<- BackupResult) {
	dbStartTime := time.Now()

	dump, err := mysqlBackup.Dump(
		client,
		dbName,
		progress,
	)
	if err != nil {
		resultsChan <- BackupResult{
			ServerName: serverName,
			Database:   dbName,
			Success:    false,
			Error:      err,
			StartTime:  dbStartTime,
			EndTime:    time.Now(),
		}
		return
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s.sql.gz", dbName, timestamp)
	fullPath := filepath.Join(serverDir, filename)

	if err := os.WriteFile(fullPath, dump, 0644); err != nil {
		resultsChan <- BackupResult{
			ServerName: serverName,
			Database:   dbName,
			Success:    false,
			Error:      err,
			StartTime:  dbStartTime,
			EndTime:    time.Now(),
		}
		return
	}

	fileInfo, _ := os.Stat(fullPath)
	resultsChan <- BackupResult{
		ServerName: serverName,
		Database:   dbName,
		Success:    true,
		StartTime:  dbStartTime,
		EndTime:    time.Now(),
		FileSize:   fileInfo.Size(),
	}
}

func backupServer(server config.Server, mysqlBackup *mysql.MySQL, progress *mpb.Progress, resultsChan chan BackupResult) {

	serverDir := filepath.Join(server.OutputPath, config.SanitizeDirectoryName(server.Name))
	if err := ensureOutputDir(serverDir); err != nil {
		resultsChan <- BackupResult{
			ServerName: server.Name,
			Success:    false,
			Error:      err,
			StartTime:  time.Now(),
			EndTime:    time.Now(),
		}
		return
	}

	client, err := ssh.NewClient(
		server.Host,
		server.Port,
		server.User,
		server.AuthType,
		server.KeyPath,
		server.Passphrase,
	)
	if err != nil {
		resultsChan <- BackupResult{
			ServerName: server.Name,
			Success:    false,
			Error:      err,
			StartTime:  time.Now(),
			EndTime:    time.Now(),
		}
		return
	}

	err = client.Connect()
	if err != nil {
		resultsChan <- BackupResult{
			ServerName: server.Name,
			Success:    false,
			Error:      err,
			StartTime:  time.Now(),
			EndTime:    time.Now(),
		}
		return
	}

	err = mysqlBackup.CreateConfigFile(client, server.Database.User, server.Database.Password, server.Database.Port)
	if err != nil {
		resultsChan <- BackupResult{
			ServerName: server.Name,
			Success:    false,
			Error:      err,
			StartTime:  time.Now(),
			EndTime:    time.Now(),
		}
		return
	}

	defer mysqlBackup.CleanupConfigFile(client)

	var databasesToBackup []string
	if server.Database.BackupAll {
		databases, err := mysqlBackup.ListDatabases(
			client,
			server.Database.User,
			server.Database.Password,
			server.Database.Port,
		)
		if err != nil {
			resultsChan <- BackupResult{
				ServerName: server.Name,
				Success:    false,
				Error:      err,
				StartTime:  time.Now(),
				EndTime:    time.Now(),
			}
			return
		}
		databasesToBackup = databases
	} else {
		databasesToBackup = []string{server.Database.Name}
	}

	dbSemaphore := make(chan struct{}, globalConfig.MaxConcurrentDatabases)

	var dbWg sync.WaitGroup
	for _, dbName := range databasesToBackup {
		dbWg.Add(1)
		go func(db string) {
			defer dbWg.Done()
			dbSemaphore <- struct{}{}
			defer func() { <-dbSemaphore }()
			backupDatabase(client, server.Name, serverDir, db, mysqlBackup, progress, resultsChan)
		}(dbName)
	}

	dbWg.Wait()
	client.Close()

	if server.RetentionDays > 0 {
		if err := cleanupOldBackups(serverDir, server.RetentionDays); err != nil {
			log.Printf("Warning: failed to cleanup old backups for %s: %v", server.Name, err)
		}
	}
}

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	globalConfig = cfg

	credManager, err := credentials.NewManager("credentials.yaml.age", cfg.PrivateKeyPath)
	if err != nil {
		log.Fatalf("Error initializing credential manager: %v", err)
	}

	if err := credManager.LoadCredentials(); err != nil {
		log.Fatalf("Error loading credentials: %v", err)
	}

	progress := mpb.New(

		mpb.WithWidth(30),
		mpb.WithRefreshRate(180*time.Millisecond),
		mpb.WithAutoRefresh(),
	)

	mysqlBackup := mysql.New()
	var wg sync.WaitGroup
	serverSemaphroe := make(chan struct{}, globalConfig.MaxConcurrentServers)
	resultsChan := make(chan BackupResult)
	startTime := time.Now()

	fmt.Println("Starting backup...")

	var results []BackupResult
	go func() {
		for result := range resultsChan {
			results = append(results, result)
		}
	}()

	for _, server := range cfg.Servers {

		serverCreds, err := credManager.GetCredential(server.CredentialsKey)
		if err != nil {
			log.Printf("Warning: failed to load credentials for %s: %v", server.Name, err)
			continue
		}

		dbCreds, err := credManager.GetCredential(server.Database.CredentialsKey)
		if err != nil {
			log.Printf("Warning: failed to load database credentials for %s: %v", server.Name, err)
		}

		serverWithCreds := server
		serverWithCreds.Passphrase = serverCreds.Passphrase
		serverWithCreds.Database.Password = dbCreds.Password

		wg.Add(1)
		go func(server config.Server) {
			defer wg.Done()
			serverSemaphroe <- struct{}{}
			defer func() { <-serverSemaphroe }()
			backupServer(server, mysqlBackup, progress, resultsChan)
		}(serverWithCreds)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	wg.Wait()
	progress.Wait()

	// Collect and process results
	var totalSuccess, totalFailure int
	var totalSize int64

	for _, result := range results {
		if result.Success {
			totalSuccess++
			totalSize += result.FileSize
			fmt.Printf("✅ %s - %s backed up successfully (%.2f MB, took %.1fs)\n",
				result.ServerName,
				result.Database,
				float64(result.FileSize)/1024/1024,
				result.EndTime.Sub(result.StartTime).Seconds(),
			)
		} else {
			totalFailure++
			fmt.Printf("❌ %s - %s failed: %v\n",
				result.ServerName,
				result.Database,
				result.Error,
			)
		}
	}

	// Print summary
	fmt.Printf("\nBackup Summary:\n")
	fmt.Printf("Total time: %s\n", time.Since(startTime).Round(time.Second))
	fmt.Printf("Successful backups: %d / %d\n", totalSuccess, totalSuccess+totalFailure)
	fmt.Printf("Failed backups: %d\n", totalFailure)
	fmt.Printf("Total backup size: %.2f MB\n", float64(totalSize)/1024/1024)

}
