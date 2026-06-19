package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestLoadProject(t *testing.T) {
	d := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	os.Chdir(d)
	if err := os.WriteFile(".agentban.yaml", []byte("api_url: https://api.example\nproject_id: abc\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadProject()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIURL != "https://api.example" || c.ProjectID != "abc" {
		t.Fatalf("config inesperada: %#v", c)
	}
}

func TestRequestExplainsIncompatibleRoute(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("404 page not found\n"))}, nil
	})}
	api := client{base: "https://api.example", token: "test", http: httpClient}
	_, err := api.request(context.Background(), http.MethodPost, "/old-route", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "go install github.com/v30-com-br/agentban-cli") {
		t.Fatalf("erro sem orientação de atualização: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
