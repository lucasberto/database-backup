package ssh

import (
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	sshClient  *ssh.Client
	Config     *ssh.ClientConfig
	Host       string
	Port       int
	Passphrase string
}

func NewClient(host string, port int, user string, authType string, authData string, passphrase string) (*Client, error) {
	var auth ssh.AuthMethod

	switch authType {
	case "key":
		key, err := os.ReadFile(authData)
		if err != nil {
			return nil, fmt.Errorf("unable to read private key: %v", err)
		}

		var signer ssh.Signer
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("unable to parse private key: %v", err)
		}

		auth = ssh.PublicKeys(signer)
	case "password":
		auth = ssh.Password(authData)
	default:
		return nil, fmt.Errorf("unsupported authentication type: %s", authType)
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return &Client{
		Config: config,
		Host:   host,
		Port:   port,
	}, nil
}

func (c *Client) Connect() error {
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", c.Host, c.Port), c.Config)
	if err != nil {
		return err
	}
	c.sshClient = client
	return nil
}

func (c *Client) GetSSHClient() *ssh.Client {
	return c.sshClient
}

func (c *Client) Close() error {
	if c.sshClient != nil {
		return c.sshClient.Close()
	}
	return nil
}
