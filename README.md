# Agentban CLI

```bash
go install github.com/v30/agentban-cli/cmd/agentban@latest
agentban auth --token agban_...
cd seu-repositorio
agentban init --project UUID
agentban run --provider codex
```

O runner exige uma branch com upstream e um worktree limpo. Cada agente deve testar, criar commit, fazer push e chamar `agentban complete`.
