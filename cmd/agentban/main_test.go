package main

import (
	"os"
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
