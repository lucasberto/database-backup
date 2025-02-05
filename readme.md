# Database Backup Tool

A Go-based tool for automated MySQL database backups over SSH with encrypted credentials and progress monitoring.

## Features

- SSH-based remote database backups
- Encrypted credentials storage using age encryption
- Concurrent backup operations
- Progress monitoring with real-time feedback
- Gzip compression for backup files
- Support for backing up multiple databases
- Detailed backup reporting

## Prerequisites

- Go 1.19 or higher
- age encryption tool (`age-keygen`)
- SSH access to your database servers (currently only supports key-based authentication)
- MySQL on remote servers (I plan to add support for other engines in the future)

## Installation

1. Clone the repository:

```bash
git clone https://github.com/lucasberto/database-backup-tool.git
cd database-backup-tool
```

2. Install dependencies:

```bash
go mod download
```

## Configuration

1. Generate encryption keys (needs age encryption tool):

```bash
age-keygen -o key.txt
```

2. Split the keys (public and private) and store them in separate files.

```bash
grep "AGE-SECRET-KEY-" key.txt > private-key.txt
grep "public key:" key.txt | sed 's/# public key: //' > public-key.txt
chmod 600 private-key.txt
rm key.txt
```

**Store the keys in a safe location**

3. Copy example configuration files to defintive ones

```bash
cp config.yaml.example config.yaml
cp credentials.yaml.example credentials.yaml
```

4. Edit config.yaml and credentials.yaml files with your settings.
5. Encrypt your credentials

```bash
go run cmd/encrypt/main.go -in credentials.yaml -out credentials.yaml.age -pubkey public-key.txt

```

6. Delete the unencrypted credentials file

```bash
rm credentials.yaml
```

## Usage

```bash
go run cmd/backup/main.go
```

The tool will:

1. Connect to each configured server
2. Create compressed MySQL dumps
3. Store backups in the specified output directory
4. Display progress bars during backup
5. Show a summary of successful and failed backups

## Backup directory structure

Backups are stored in the following format:

```
/output_path/
└── server_name/
    └── database_name_YYYY-MM-DD_HH-mm-ss.sql.gz
```

## Security notes

- Keep your private key secure and never commit it to version control
- Use strong passwords for both SSH and database access
- Consider using SSH keys with passphrases for additional security
- Regularly rotate your database passwords
