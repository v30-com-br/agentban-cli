package main

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestPublishChangesCommitsAndPushes(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	repo := filepath.Join(root, "repo")
	runGit(t, root, "init", "--bare", remote)
	runGit(t, root, "init", "-b", "main", repo)
	runGit(t, repo, "config", "user.name", "Agentban Test")
	runGit(t, repo, "config", "user.email", "agentban@example.com")
	runGit(t, repo, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(repo, "change.txt"), []byte("done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	if err := publishChanges("12345678-abcd"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pushedState(); err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, repo, "log", "-1", "--pretty=%s"); got != "agentban: complete 12345678" {
		t.Fatalf("mensagem de commit inesperada: %q", got)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
