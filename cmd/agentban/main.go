package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const defaultAPI = "https://api.agentban.v30.com.br"

type projectConfig struct{ APIURL, ProjectID string }
type credentials struct {
	Token string `json:"token"`
}
type ticket struct {
	ID, ProjectID, Title, Content, Status string
	Position                              int64
}
type execution struct {
	ID, TicketID, Provider, Status string
	LeaseExpiresAt                 time.Time
}
type claim struct {
	Ticket       ticket    `json:"ticket"`
	Execution    execution `json:"execution"`
	LeaseToken   string    `json:"leaseToken"`
	LeaseSeconds int       `json:"leaseSeconds"`
}
type client struct {
	base, token string
	http        *http.Client
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "auth":
		err = auth(os.Args[2:])
	case "init":
		err = initProject(os.Args[2:])
	case "run":
		err = run(os.Args[2:])
	case "comment":
		err = comment(os.Args[2:])
	case "complete":
		err = complete(os.Args[2:])
	case "fail":
		err = failTicket(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentban:", err)
		os.Exit(1)
	}
}
func usage() { fmt.Fprintln(os.Stderr, "uso: agentban <auth|init|run|comment|complete|fail>") }

func configDir() (string, error) {
	if d := os.Getenv("AGENTBAN_CONFIG_DIR"); d != "" {
		return d, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "agentban"), nil
}
func auth(args []string) error {
	fs := flag.NewFlagSet("auth", flag.ContinueOnError)
	token := fs.String("token", "", "token global")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token == "" {
		return errors.New("use --token agban_...")
	}
	d, err := configDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(d, 0700); err != nil {
		return err
	}
	body, _ := json.Marshal(credentials{Token: *token})
	return os.WriteFile(filepath.Join(d, "credentials.json"), body, 0600)
}
func initProject(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	id := fs.String("project", "", "ID do projeto")
	api := fs.String("api", defaultAPI, "URL da API")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("use --project <uuid>")
	}
	body := fmt.Sprintf("api_url: %s\nproject_id: %s\n", strings.TrimRight(*api, "/"), *id)
	return os.WriteFile(".agentban.yaml", []byte(body), 0644)
}
func loadProject() (projectConfig, error) {
	b, err := os.ReadFile(".agentban.yaml")
	if err != nil {
		return projectConfig{}, fmt.Errorf("leia .agentban.yaml: %w", err)
	}
	var c projectConfig
	for _, line := range strings.Split(string(b), "\n") {
		p := strings.SplitN(line, ":", 2)
		if len(p) != 2 {
			continue
		}
		switch strings.TrimSpace(p[0]) {
		case "api_url":
			c.APIURL = strings.TrimSpace(p[1])
		case "project_id":
			c.ProjectID = strings.TrimSpace(p[1])
		}
	}
	if c.APIURL == "" || c.ProjectID == "" {
		return c, errors.New(".agentban.yaml inválido")
	}
	return c, nil
}
func loadClient() (client, projectConfig, error) {
	p, err := loadProject()
	if err != nil {
		return client{}, p, err
	}
	d, err := configDir()
	if err != nil {
		return client{}, p, err
	}
	b, err := os.ReadFile(filepath.Join(d, "credentials.json"))
	if err != nil {
		return client{}, p, errors.New("credenciais ausentes; execute agentban auth --token ...")
	}
	var cr credentials
	if json.Unmarshal(b, &cr) != nil || cr.Token == "" {
		return client{}, p, errors.New("credenciais inválidas")
	}
	return client{base: strings.TrimRight(p.APIURL, "/"), token: cr.Token, http: &http.Client{Timeout: 30 * time.Second}}, p, nil
}
func (c client) request(ctx context.Context, method, path string, input, output any, lease string) (int, error) {
	var body io.Reader
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if lease != "" {
		req.Header.Set("X-Agentban-Lease", lease)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return res.StatusCode, fmt.Errorf("API %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if output != nil && res.StatusCode != 204 {
		if err = json.NewDecoder(res.Body).Decode(output); err != nil {
			return res.StatusCode, err
		}
	}
	return res.StatusCode, nil
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func ensureClean() error {
	s, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if s != "" {
		return errors.New("worktree possui alterações locais")
	}
	if _, err = git("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err != nil {
		return errors.New("branch atual não possui upstream")
	}
	return nil
}
func pushedState() (sha, branch string, err error) {
	if err = ensureClean(); err != nil {
		return
	}
	sha, err = git("rev-parse", "HEAD")
	if err != nil {
		return
	}
	remote, er := git("rev-parse", "@{u}")
	if er != nil {
		err = er
		return
	}
	if sha != remote {
		err = errors.New("HEAD ainda não foi enviado ao upstream")
		return
	}
	branch, err = git("branch", "--show-current")
	return
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	provider := fs.String("provider", "codex", "codex ou claude")
	name := fs.String("name", hostname(), "nome do agente")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *provider != "codex" && *provider != "claude" {
		return errors.New("provider deve ser codex ou claude")
	}
	api, p, err := loadClient()
	if err != nil {
		return err
	}
	for {
		var cl claim
		status, err := api.request(context.Background(), "POST", "/v1/agent/projects/"+p.ProjectID+"/tickets/claim", map[string]string{"provider": *provider, "agentName": *name}, &cl, "")
		if err != nil {
			return err
		}
		if status == 204 {
			fmt.Println("Fila vazia.")
			return nil
		}
		fmt.Printf("[%s] %s\n", cl.Ticket.ID, cl.Ticket.Title)
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); heartbeat(ctx, api, cl) }()
		err = invoke(*provider, cl)
		cancel()
		wg.Wait()
		var current execution
		_, getErr := api.request(context.Background(), "GET", "/v1/agent/executions/"+cl.Execution.ID, nil, &current, cl.LeaseToken)
		if getErr != nil {
			return getErr
		}
		if current.Status == "running" {
			reason := "Agente encerrou sem tramitar o ticket"
			if err != nil {
				reason = err.Error()
			}
			_, failErr := api.request(context.Background(), "POST", "/v1/agent/executions/"+cl.Execution.ID+"/fail", map[string]string{"error": reason}, nil, cl.LeaseToken)
			if failErr != nil {
				return failErr
			}
		}
	}
}
func heartbeat(ctx context.Context, api client, cl claim) {
	interval := time.Duration(cl.LeaseSeconds/3) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = api.request(ctx, "POST", "/v1/agent/executions/"+cl.Execution.ID, nil, nil, cl.LeaseToken)
		}
	}
}
func invoke(provider string, cl claim) error {
	exe, _ := os.Executable()
	prompt := fmt.Sprintf(`Você está executando o ticket Agentban %s.

Título: %s

Conteúdo:
%s

Implemente completamente no repositório atual. Preserve a branch atual. Execute testes proporcionais à mudança, faça commit e git push. Durante o trabalho, use:
  %s comment --body "mensagem"
Ao concluir, depois do push confirmado, use:
  %s complete --comment "resumo"
Se não puder concluir, use:
  %s fail --error "motivo"
Não conclua o ticket antes do commit estar publicado no upstream.`, cl.Ticket.ID, cl.Ticket.Title, cl.Ticket.Content, exe, exe, exe)
	var cmd *exec.Cmd
	if provider == "codex" {
		cmd = exec.Command("codex", "exec", "-s", "workspace-write", "--skip-git-repo-check", "-C", ".", prompt)
	} else {
		cmd = exec.Command("claude", "-p", "--verbose", "--permission-mode", "acceptEdits", prompt)
	}
	cmd.Env = append(os.Environ(), "AGENTBAN_EXECUTION_ID="+cl.Execution.ID, "AGENTBAN_LEASE_TOKEN="+cl.LeaseToken)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func contextClient() (client, string, string, error) {
	api, _, err := loadClient()
	if err != nil {
		return api, "", "", err
	}
	id := os.Getenv("AGENTBAN_EXECUTION_ID")
	lease := os.Getenv("AGENTBAN_LEASE_TOKEN")
	if id == "" || lease == "" {
		return api, "", "", errors.New("comando disponível apenas dentro de agentban run")
	}
	return api, id, lease, nil
}
func comment(args []string) error {
	fs := flag.NewFlagSet("comment", flag.ContinueOnError)
	body := fs.String("body", "", "comentário")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*body) == "" {
		return errors.New("use --body")
	}
	api, id, lease, err := contextClient()
	if err != nil {
		return err
	}
	_, err = api.request(context.Background(), "POST", "/v1/agent/executions/"+id+"/comments", map[string]string{"body": *body}, nil, lease)
	return err
}
func complete(args []string) error {
	fs := flag.NewFlagSet("complete", flag.ContinueOnError)
	msg := fs.String("comment", "", "resumo")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sha, branch, err := pushedState()
	if err != nil {
		return err
	}
	api, id, lease, err := contextClient()
	if err != nil {
		return err
	}
	_, err = api.request(context.Background(), "POST", "/v1/agent/executions/"+id+"/complete", map[string]string{"commitSHA": sha, "branch": branch, "comment": *msg}, nil, lease)
	return err
}
func failTicket(args []string) error {
	fs := flag.NewFlagSet("fail", flag.ContinueOnError)
	reason := fs.String("error", "", "motivo")
	comment := fs.String("comment", "", "detalhes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*reason) == "" {
		return errors.New("use --error")
	}
	api, id, lease, err := contextClient()
	if err != nil {
		return err
	}
	_, err = api.request(context.Background(), "POST", "/v1/agent/executions/"+id+"/fail", map[string]string{"error": *reason, "comment": *comment}, nil, lease)
	return err
}
func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return runtime.GOOS
	}
	return h
}
