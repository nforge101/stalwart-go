// Command basic shows how to construct a stalwart Client from environment
// variables. It does not contact the Stalwart server and performs no
// management operation.
//
// Environment variables:
//
//	STALWART_URL      base URL of the Stalwart server, for example https://mail.example.com
//	STALWART_API_KEY  Stalwart API key (begins with API_)
//
// The API key is never printed.
package main

import (
	"fmt"
	"io"
	"os"

	stalwart "github.com/nforge101/stalwart-go"
)

const (
	envURL    = "STALWART_URL"
	envAPIKey = "STALWART_API_KEY"
)

func main() {
	if err := run(os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run builds a Client from the values returned by getenv and writes a safe
// description of it to stdout. Errors and output never contain the API key.
func run(getenv func(string) string, stdout io.Writer) error {
	baseURL := getenv(envURL)
	if baseURL == "" {
		return fmt.Errorf("%s is not set", envURL)
	}
	apiKey := getenv(envAPIKey)
	if apiKey == "" {
		return fmt.Errorf("%s is not set", envAPIKey)
	}

	client, err := stalwart.NewClient(baseURL, stalwart.WithBearerToken(apiKey))
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(stdout, client)
	return err
}
