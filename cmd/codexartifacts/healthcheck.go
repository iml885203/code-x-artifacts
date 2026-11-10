package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

func listenAddress() string { return cmp.Or(os.Getenv("PICTOR_ADDR"), "127.0.0.1:8080") }

// A probe runs without opening SQLite or loading accounts/provider settings.
// Wildcard listeners are reached through loopback; proxy environment variables
// and redirects cannot turn a local liveness check into a different request.
func healthcheck(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("invalid PICTOR_ADDR for healthcheck")
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	target := url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/healthz"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return errors.New("invalid healthcheck target")
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("healthcheck request failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned status %d", response.StatusCode)
	}
	return nil
}
