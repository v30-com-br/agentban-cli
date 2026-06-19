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
	"runtime/debug"
	"strings"
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
type claim struct {
	Ticket ticket `json:"ticket"`
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
	case "version":
		fmt.Println(buildVersion())
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentban:", err)
		os.Exit(1)
	}
}
func usage() { fmt.Fprintln(os.Stderr, "uso: agentban <auth|init|run|comment|complete|fail|version>") }

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}

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
func (c client) request(ctx context.Context, method, path string, input, output any) (int, error) {
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
	res, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		if res.StatusCode == http.StatusNotFound && strings.TrimSpace(string(b)) == "404 page not found" {
			return res.StatusCode, errors.New("API 404: rota incompatível; atualize com `go install github.com/v30-com-br/agentban-cli/cmd/agentban@latest`")
		}
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

func publishChanges(ticketID string) error {
	status, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		if _, err = git("add", "-A"); err != nil {
			return err
		}
		shortID := ticketID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		if _, err = git("commit", "-m", "agentban: complete "+shortID); err != nil {
			return err
		}
	}
	branch, err := git("branch", "--show-current")
	if err != nil || branch == "" {
		return errors.New("não foi possível determinar a branch atual")
	}
	if _, err = git("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		_, err = git("push")
		return err
	}
	remotes, err := git("remote")
	if err != nil {
		return err
	}
	available := strings.Fields(remotes)
	remote := "origin"
	if len(available) == 1 {
		remote = available[0]
	} else if !contains(available, remote) {
		return errors.New("branch sem upstream e remote origin ausente")
	}
	_, err = git("push", "-u", remote, branch)
	return err
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	provider := fs.String("provider", "codex", "codex ou claude")
	name := fs.String("name", hostname(), "nome do agente")
	verbose := fs.Bool("verbose", false, "exibe a saída completa do agente")
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
		status, err := api.request(context.Background(), "POST", "/v1/agent/projects/"+p.ProjectID+"/tickets/claim", map[string]string{"provider": *provider, "agentName": *name}, &cl)
		if err != nil {
			return err
		}
		if status == 204 {
			fmt.Println("Fila vazia.")
			return nil
		}
		fmt.Printf("[%s] %s\n", cl.Ticket.ID, cl.Ticket.Title)
		err = invoke(*provider, cl, *verbose)
		if err == nil {
			err = publishChanges(cl.Ticket.ID)
		}
		if err == nil {
			sha, branch, stateErr := pushedState()
			if stateErr != nil {
				err = stateErr
			} else {
				_, err = api.request(context.Background(), "POST", "/v1/agent/tickets/"+cl.Ticket.ID+"/complete", map[string]string{"commitSHA": sha, "branch": branch, "comment": "Implementação concluída por " + *provider}, nil)
			}
		}
		if err != nil {
			if _, failErr := api.request(context.Background(), "POST", "/v1/agent/tickets/"+cl.Ticket.ID+"/fail", map[string]string{"error": err.Error()}, nil); failErr != nil {
				return fmt.Errorf("%v; falha ao atualizar ticket: %w", err, failErr)
			}
			return err
		}
	}
}
func invoke(provider string, cl claim, verbose bool) error {
	exe, _ := os.Executable()
	prompt := fmt.Sprintf(`Você está executando o ticket Agentban %s.

Título: %s

Conteúdo:
%s

Implemente completamente no repositório atual. Preserve a branch atual e execute testes proporcionais à mudança. Você pode usar todas as ferramentas e funcionalidades disponíveis. Durante o trabalho, use:
  %s comment --body "mensagem"
Ao terminar, apenas encerre normalmente. O Agentban fará commit e push de alterações remanescentes e só então concluirá o ticket.`, cl.Ticket.ID, cl.Ticket.Title, cl.Ticket.Content, exe)
	var cmd *exec.Cmd
	if provider == "codex" {
		cmd = exec.Command("codex", "exec", "--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check", "-C", ".", prompt)
	} else {
		cmd = exec.Command("claude", "-p", "--permission-mode", "bypassPermissions", prompt)
	}
	cmd.Env = append(os.Environ(), "AGENTBAN_TICKET_ID="+cl.Ticket.ID)
	// ponytail: sem stdin — codex/claude recebem o prompt por argumento; stdin aberto faz o codex travar em "Reading additional input from stdin..."
	if provider != "codex" || verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s encerrou com erro: %w", provider, err)
	}
	return nil
}
func contextClient() (client, string, error) {
	api, _, err := loadClient()
	if err != nil {
		return api, "", err
	}
	id := os.Getenv("AGENTBAN_TICKET_ID")
	if id == "" {
		return api, "", errors.New("comando disponível apenas dentro de agentban run")
	}
	return api, id, nil
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
	api, id, err := contextClient()
	if err != nil {
		return err
	}
	_, err = api.request(context.Background(), "POST", "/v1/agent/tickets/"+id+"/comments", map[string]string{"body": *body}, nil)
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
	api, id, err := contextClient()
	if err != nil {
		return err
	}
	_, err = api.request(context.Background(), "POST", "/v1/agent/tickets/"+id+"/complete", map[string]string{"commitSHA": sha, "branch": branch, "comment": *msg}, nil)
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
	api, id, err := contextClient()
	if err != nil {
		return err
	}
	_, err = api.request(context.Background(), "POST", "/v1/agent/tickets/"+id+"/fail", map[string]string{"error": *reason, "comment": *comment}, nil)
	return err
}
func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return runtime.GOOS
	}
	return h
}
