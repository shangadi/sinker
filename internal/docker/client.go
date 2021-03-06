package docker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

// Client is a Docker client with a logger
type Client struct {
	DockerClient *client.Client
	Logger       *log.Logger
}

// NewClient returns a new Docker client
func NewClient(logger *log.Logger) (Client, error) {
	retry.DefaultDelay = 5 * time.Second
	retry.DefaultAttempts = 3

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return Client{}, fmt.Errorf("new docker client: %w", err)
	}

	client := Client{
		DockerClient: dockerClient,
		Logger:       logger,
	}

	return client, nil
}

// RegistryPath is a registry path for a docker image
type RegistryPath string

// Digest is the digest in the registry path, if one exists
func (r RegistryPath) Digest() string {
	if !strings.Contains(string(r), "@") {
		return ""
	}

	digestTokens := strings.Split(string(r), "@")

	return digestTokens[1]
}

// Tag is the tag in the registry path, if one exists
func (r RegistryPath) Tag() string {
	if strings.Contains(string(r), "@") || !strings.Contains(string(r), ":") {
		return ""
	}

	tagTokens := strings.Split(string(r), ":")

	return tagTokens[1]
}

// Host is the host in the registry path
func (r RegistryPath) Host() string {
	host := string(r)

	if r.Tag() != "" {
		host = strings.ReplaceAll(host, ":"+r.Tag(), "")
	}

	if !strings.Contains(host, ".") {
		return ""
	}

	hostTokens := strings.Split(string(r), "/")

	return hostTokens[0]
}

// Repository is the repository in the registry path
func (r RegistryPath) Repository() string {
	repository := string(r)

	if r.Tag() != "" {
		repository = strings.ReplaceAll(repository, ":"+r.Tag(), "")
	}

	if r.Digest() != "" {
		repository = strings.ReplaceAll(repository, "@"+r.Digest(), "")
	}

	if r.Host() != "" {
		repository = strings.ReplaceAll(repository, r.Host(), "")
	}

	repository = strings.TrimLeft(repository, "/")

	return repository
}

// ProgressDetail is the current state of pushing or pulling an image (in Bytes)
type ProgressDetail struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}

// Status is the status output from the Docker client
type Status struct {
	Message        string         `json:"status"`
	ID             string         `json:"id"`
	ProgressDetail ProgressDetail `json:"progressDetail"`
}

// GetMessage returns a human friendly message from parsing the status message
func (s Status) GetMessage() string {
	if strings.Contains(s.Message, "Pulling from") || strings.Contains(s.Message, "The push refers to") {
		return "Started"
	}

	if s.ProgressDetail.Total > 0 {
		return fmt.Sprintf("Processing %vB of %vB", s.ProgressDetail.Current, s.ProgressDetail.Total)
	}

	return "Processing"
}

func waitForScannerComplete(logger *log.Logger, clientScanner *bufio.Scanner, image string, command string) error {
	type clientErrorMessage struct {
		Error string `json:"error"`
	}

	var errorMessage clientErrorMessage
	var status Status

	var scans int
	for clientScanner.Scan() {
		if err := json.Unmarshal(clientScanner.Bytes(), &status); err != nil {
			return fmt.Errorf("unmarshal status: %w", err)
		}

		if err := json.Unmarshal(clientScanner.Bytes(), &errorMessage); err != nil {
			return fmt.Errorf("unmarshal error: %w", err)
		}

		if errorMessage.Error != "" {
			return fmt.Errorf("returned error: %s", errorMessage.Error)
		}

		// Serves as makeshift polling to occasionally print the status of the Docker command.
		if scans%25 == 0 {
			logger.Printf("[%s] %s (%s)", command, image, status.GetMessage())
		}

		scans++
	}

	if clientScanner.Err() != nil {
		return fmt.Errorf("scanner: %w", clientScanner.Err())
	}

	logger.Printf("[%s] %s complete.", command, image)

	return nil
}
