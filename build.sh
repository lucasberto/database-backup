#!/bin/bash
GOOS=linux GOARCH=amd64 go build -o backup-tool-linux cmd/backup/main.go
GOOS=windows GOARCH=amd64 go build -o backup-tool-windows.exe cmd/backup/main.go
