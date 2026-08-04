#!/bin/bash
GOOS=linux GOARCH=amd64 go build -o backup-tool-linux ./cmd/backup
GOOS=windows GOARCH=amd64 go build -o backup-tool-windows.exe ./cmd/backup
