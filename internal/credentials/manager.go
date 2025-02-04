package credentials

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
	"gopkg.in/yaml.v3"
)

type Credentials struct {
	Credentials map[string]ServerCredentials `yaml:"credentials"`
}

type ServerCredentials struct {
	Passphrase string `yaml:"passphrase,omitempty"`
	Password   string `yaml:"password,omitempty"`
}

type Manager struct {
	credsFile   string
	keyFile     string
	key         string
	isPublic    bool
	credentials *Credentials
}

func NewManager(credsFile, privateKeyPath string) (*Manager, error) {
	privateKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}

	return &Manager{
		credsFile: credsFile,
		keyFile:   privateKeyPath,
		key:       privateKey,
		isPublic:  false,
	}, nil
}

func NewEncryptionManager(credsFile, publicKeyPath string) (*Manager, error) {
	publicKey, err := loadPublicKey(publicKeyPath)
	if err != nil {
		return nil, err
	}

	return &Manager{
		credsFile: credsFile,
		keyFile:   publicKeyPath,
		key:       publicKey,
		isPublic:  true,
	}, nil
}

func (m *Manager) LoadCredentials() error {
	if m.isPublic {
		return fmt.Errorf("cannot decrypt with public key")
	}

	encrypted, err := os.ReadFile(m.credsFile)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file: %v", err)
	}

	identity, err := age.ParseX25519Identity(m.key)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %v", err)
	}

	decrypted, err := decrypt(encrypted, identity)
	if err != nil {
		return fmt.Errorf("failed to decrypt credentials: %v", err)
	}

	var creds Credentials
	if err := yaml.Unmarshal(decrypted, &creds); err != nil {
		return fmt.Errorf("failed to parse decrypted credentials: %v", err)
	}

	m.credentials = &creds
	return nil
}

func (m *Manager) GetCredential(key string) (ServerCredentials, error) {
	if m.credentials == nil {
		return ServerCredentials{}, fmt.Errorf("credentials not loaded")
	}

	cred, exists := m.credentials.Credentials[key]
	if !exists {
		return ServerCredentials{}, fmt.Errorf("credential not found: %s", key)
	}

	return cred, nil
}

func (m *Manager) SaveCredentials(creds *Credentials) error {
	data, err := yaml.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %v", err)
	}

	recipient, err := age.ParseX25519Recipient(m.key)
	if err != nil {
		return fmt.Errorf("failed to parse key: %v", err)
	}

	encrypted, err := encrypt(data, recipient)
	if err != nil {
		return fmt.Errorf("failed to encrypt credentials: %v", err)
	}

	return os.WriteFile(m.credsFile, encrypted, 0600)
}

func encrypt(data []byte, recipient age.Recipient) ([]byte, error) {
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, err
	}

	if _, err := w.Write(data); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func decrypt(encrypted []byte, identity age.Identity) ([]byte, error) {
	reader, err := age.Decrypt(bytes.NewReader(encrypted), identity)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(reader)
}

func loadPrivateKey(keyFile string) (string, error) {
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read private key file: %v", err)
	}

	key := strings.TrimSpace(string(data))
	if !strings.HasPrefix(key, "AGE-SECRET-KEY-") {
		return "", fmt.Errorf("invalid private key format")
	}

	return key, nil
}

func loadPublicKey(keyFile string) (string, error) {
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read public key file: %v", err)
	}

	key := strings.TrimSpace(string(data))
	if !strings.HasPrefix(key, "age1") {
		return "", fmt.Errorf("invalid public key format")
	}

	return key, nil
}

func (m *Manager) EncryptFile(inputFile string) error {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %v", err)
	}

	// Parse YAML to validate format
	var creds Credentials
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("invalid credentials format: %v", err)
	}

	recipient, err := age.ParseX25519Recipient(m.key)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %v", err)
	}

	encrypted, err := encrypt(data, recipient)
	if err != nil {
		return fmt.Errorf("failed to encrypt data: %v", err)
	}

	return os.WriteFile(m.credsFile, encrypted, 0600)
}
