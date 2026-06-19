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

Codex e Claude são iniciados sem limitação de ferramentas, com saída detalhada em tempo real, e recebem a instrução de criar o commit e fazer `git push`. O CLI não executa nem valida operações Git: quando o agente encerra com sucesso, conclui o ticket e busca o próximo.

Para conferir a versão instalada:

```bash
agentban version
```
