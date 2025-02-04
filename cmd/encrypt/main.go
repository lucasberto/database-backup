package main

import (
	"flag"
	"log"

	"github.com/lucasberto/database-backup-tool/internal/credentials"
)

func main() {
	inputFile := flag.String("in", "credentials.yaml", "Input credentials file")
	outputFile := flag.String("out", "credentials.yaml.age", "Output encrypted file")
	publicKeyFile := flag.String("pubkey", "public-key.txt", "Age public key file")
	flag.Parse()

	credManager, err := credentials.NewEncryptionManager(*outputFile, *publicKeyFile)
	if err != nil {
		log.Fatalf("Error creating encryption manager: %v", err)
	}

	if err := credManager.EncryptFile(*inputFile); err != nil {
		log.Fatalf("Error encrypting credentials: %v", err)
	}

	log.Printf("Successfully encrypted credentials to %s", *outputFile)
}
