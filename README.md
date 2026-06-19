# Agentban CLI

## Instalação

```bash
# baixa, compila e joga o binário no PATH (~/go/bin)
gh repo clone v30-com-br/agentban-cli /tmp/agentban-cli
go build -o "$(go env GOPATH)/bin/agentban" /tmp/agentban-cli/cmd/agentban

# garante ~/go/bin no PATH (uma vez)
grep -q 'go/bin' ~/.zshrc || echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

> `go install github.com/...@latest` não funciona: o repo é privado e o `go.mod`
> declara `github.com/v30/agentban-cli` (≠ caminho do repo `v30-com-br`). Use o build acima.

## Uso

```bash
agentban auth --token agban_...
cd seu-repositorio
agentban init --project UUID
agentban run --provider codex
```

`run` não exige git limpo nem upstream. Cada agente deve testar, criar commit, fazer push e chamar `agentban complete`.
