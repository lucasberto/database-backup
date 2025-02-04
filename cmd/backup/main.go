package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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

func backupDatabase(client *ssh.Client, serverName, serverDir, dbName string, dbConfig config.Database, mysqlBackup *mysql.MySQL, progress *mpb.Progress, resultsChan chan<- BackupResult) {
	dbStartTime := time.Now()

	dump, err := mysqlBackup.Dump(
		client,
		dbName,
		dbConfig.User,
		dbConfig.Password,
		dbConfig.Port,
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
			backupDatabase(client, server.Name, serverDir, db, server.Database, mysqlBackup, progress, resultsChan)
		}(dbName)
	}

	dbWg.Wait()
	client.Close()
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
		mpb.WithWidth(64),
		mpb.WithRefreshRate(500*time.Millisecond),
		mpb.WithAutoRefresh(),
	)

	mysqlBackup := mysql.New()
	var wg sync.WaitGroup
	serverSemaphroe := make(chan struct{}, globalConfig.MaxConcurrentServers)
	resultsChan := make(chan BackupResult, 1000)
	startTime := time.Now()

	fmt.Println("Starting backup...")

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

	for result := range resultsChan {
		if result.Success {
			totalSuccess++
			totalSize += result.FileSize
			fmt.Printf("✅ %s - %s backed up successfully (%.2f MB, took %s)\n",
				result.ServerName,
				result.Database,
				float64(result.FileSize)/1024/1024,
				result.EndTime.Sub(result.StartTime),
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
	fmt.Printf("Total time: %s\n", time.Since(startTime))
	fmt.Printf("Successful backups: %d\n", totalSuccess)
	fmt.Printf("Failed backups: %d\n", totalFailure)
	fmt.Printf("Total backup size: %.2f MB\n", float64(totalSize)/1024/1024)

}
