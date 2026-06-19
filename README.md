# Agentban CLI

## Instalação

```bash
go install github.com/v30-com-br/agentban-cli/cmd/agentban@latest

# Se necessário, adicione os binários Go ao PATH:
grep -q 'go/bin' ~/.zshrc || echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

Requisitos: Go instalado e ao menos um dos agentes disponível no `PATH`: [Codex CLI](https://github.com/openai/codex) ou [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

## Uso

```bash
agentban auth --token agban_...
cd seu-repositorio
agentban init --project UUID
agentban run --provider codex
```

`run` não exige git limpo nem upstream. Codex e Claude são iniciados sem limitação de ferramentas; ao final, o CLI cria um commit para alterações remanescentes, configura o upstream quando necessário, faz push e só então conclui o ticket. A saída do Codex fica silenciosa por padrão; use `--verbose` para acompanhar todos os detalhes.

Para conferir a versão instalada:

```bash
agentban version
```
