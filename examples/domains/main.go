// Command domains lists the domains on a Stalwart server. It is read-only: it
// makes one request and changes nothing.
//
// Environment variables:
//
//	STALWART_URL      base URL of the Stalwart server, for example https://mail.example.com
//	STALWART_API_KEY  Stalwart API key (begins with API_) that holds sysDomainGet
//
// The API key is never printed.
package main

import (
	"context"
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

// run lists the domains on the server named by the environment and writes one
// domain name per line to stdout. extra options are applied after the
// environment settings; main passes none.
//
// Errors never contain the API key, because the SDK redacts it.
func run(getenv func(string) string, stdout io.Writer, extra ...stalwart.Option) error {
	baseURL := getenv(envURL)
	if baseURL == "" {
		return fmt.Errorf("%s is not set", envURL)
	}
	apiKey := getenv(envAPIKey)
	if apiKey == "" {
		return fmt.Errorf("%s is not set", envAPIKey)
	}

	opts := append([]stalwart.Option{stalwart.WithBearerToken(apiKey)}, extra...)
	client, err := stalwart.NewClient(baseURL, opts...)
	if err != nil {
		return err
	}

	domains, err := client.Domains().List(context.Background())
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	if _, err := fmt.Fprintf(stdout, "%d domain(s)\n", len(domains)); err != nil {
		return err
	}
	for _, d := range domains {
		if _, err := fmt.Fprintln(stdout, d.Name); err != nil {
			return err
		}
	}
	return nil
}
