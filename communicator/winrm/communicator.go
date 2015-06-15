package winrm

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/masterzen/winrm/winrm"
	"github.com/mitchellh/packer/packer"
	"github.com/packer-community/winrmcp/winrmcp"

	// This import is a bit strange, but it's needed so `make updatedeps`
	// can see and download it
	_ "github.com/dylanmei/winrmtest"
)

// Communicator represents the WinRM communicator
type Communicator struct {
	config   *Config
	client   *winrm.Client
	endpoint *winrm.Endpoint
}

// New creates a new communicator implementation over WinRM.
func New(config *Config) (*Communicator, error) {
	endpoint := &winrm.Endpoint{
		Host: config.Host,
		Port: config.Port,

		/*
			TODO
			HTTPS:    connInfo.HTTPS,
			Insecure: connInfo.Insecure,
			CACert:   connInfo.CACert,
		*/
	}

	// Create the client
	params := winrm.DefaultParameters()
	params.Timeout = formatDuration(config.Timeout)
	client, err := winrm.NewClientWithParameters(
		endpoint, config.Username, config.Password, params)
	if err != nil {
		return nil, err
	}

	// Create the shell to verify the connection
	log.Printf("[DEBUG] connecting to remote shell using WinRM")
	shell, err := client.CreateShell()
	if err != nil {
		log.Printf("[ERROR] connection error: %s", err)
		return nil, err
	}

	if err := shell.Close(); err != nil {
		log.Printf("[ERROR] error closing connection: %s", err)
		return nil, err
	}

	return &Communicator{
		config:   config,
		client:   client,
		endpoint: endpoint,
	}, nil
}

// Start implementation of communicator.Communicator interface
func (c *Communicator) Start(rc *packer.RemoteCmd) error {
	shell, err := c.client.CreateShell()
	if err != nil {
		return err
	}

	log.Printf("[INFO] starting remote command: %s", rc.Command)
	cmd, err := shell.Execute(rc.Command)
	if err != nil {
		return err
	}

	go runCommand(shell, cmd, rc)
	return nil
}

func runCommand(shell *winrm.Shell, cmd *winrm.Command, rc *packer.RemoteCmd) {
	defer shell.Close()

	go io.Copy(rc.Stdout, cmd.Stdout)
	go io.Copy(rc.Stderr, cmd.Stderr)

	cmd.Wait()
	rc.SetExited(cmd.ExitCode())
}

// Upload implementation of communicator.Communicator interface
func (c *Communicator) Upload(path string, input io.Reader, _ *os.FileInfo) error {
	wcp, err := c.newCopyClient()
	if err != nil {
		return err
	}
	log.Printf("Uploading file to '%s'", path)
	return wcp.Write(path, input)
}

// UploadDir implementation of communicator.Communicator interface
func (c *Communicator) UploadDir(dst string, src string, exclude []string) error {
	log.Printf("Uploading dir '%s' to '%s'", src, dst)
	wcp, err := c.newCopyClient()
	if err != nil {
		return err
	}
	return wcp.Copy(src, dst)
}

func (c *Communicator) Download(src string, dst io.Writer) error {
	return fmt.Errorf("WinRM doesn't support download.")
}

func (c *Communicator) newCopyClient() (*winrmcp.Winrmcp, error) {
	addr := fmt.Sprintf("%s:%d", c.endpoint.Host, c.endpoint.Port)
	return winrmcp.New(addr, &winrmcp.Config{
		Auth: winrmcp.Auth{
			User:     c.config.Username,
			Password: c.config.Password,
		},
		OperationTimeout:      c.config.Timeout,
		MaxOperationsPerShell: 15, // lowest common denominator
	})
}