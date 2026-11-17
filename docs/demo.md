# Roteiro de demonstração — FIAP X (máximo 10 minutos)

**Objetivo:** mostrar todos os requisitos funcionais e evidências de entrega, sem depender de infraestrutura AWS.

| Minutos | Demonstração sugerida |
| --- | --- |
| 0:00–1:00 | Diagrama da arquitetura; dois executáveis Go, PostgreSQL, S3, RabbitMQ e responsabilidades |
| 1:00–2:00 | Abrir http://localhost:8080/, criar usuário e autenticar; explicar armazenamento seguro das senhas |
| 2:00–4:00 | Enviar dois vídeos, mostrar `queued → processing → completed`, `docker compose ps` com dois workers e baixar ZIP com PNGs |
| 4:00–5:00 | Abrir sessão de outro usuário: não visualizar job nem notificações de outra conta |
| 5:00–6:30 | Mostrar erro: enviar `.mp4` inválido; conferir `failed`, aviso in-app e email capturado em Mailpit (localhost:8025) |
| 6:30–8:00 | Explicar outbox transacional e testes E2E de broker offline/recuperação de lease; mostrar run de CI bem-sucedido |
| 8:00–9:00 | Abrir Grafana (localhost:3000/d/fiapx-operations), mostrar status dos jobs, consumidores, fila e falhas; apontar regras de alerta no Prometheus |
| 9:00–9:40 | Mostrar CI/CD aprovado, resumo de cobertura e artifact `go-coverage-report`, publicação GHCR e documentação; encerrar com links do GitHub |

## Preparar o ambiente

```bash
bash scripts/deploy-local.sh
docker compose -f docker-compose.yml -f docker-compose.mail.yml --profile monitoring ps
```

Abra a caixa de emails de demonstração em http://localhost:8025/ (Mailpit, sem credenciais). Abra a interface em http://localhost:8080/ e o dashboard Grafana em http://localhost:3000/d/fiapx-operations (admin / senha local padrão). Tenha **um vídeo MP4 curto válido** e **um arquivo de texto salvo como `invalido.mp4`**. Abra o RabbitMQ Management em http://localhost:15672 (somente se for útil). Recarregue a página para demonstrar novo login; a sessão é mantida em memória.

Para provar recuperação de maneira determinística, prefira mostrar a execução aprovada de `scripts/smoke-intake.sh` e `scripts/smoke-processing.sh` no GitHub Actions. Os scripts E2E apagam seus volumes de teste e não devem ser executados no ambiente com dados da demonstração.

## Conferência de entrega

- Repositório público ou acessível ao avaliador; README com todos os comandos.
- Runs aprovados de **Go build/testes e cobertura**, **Docker** e **quatro E2E**, incluindo monitoramento. No job Go, confirmar o artifact `go-coverage-report`.
- Job `Publish verified API and worker images` na execução do CI do `main` correspondente à versão entregue: conferir o sucesso e as imagens no GHCR; não afirmar publicação concluída sem verificar o run.
- Gravar vídeo de **até 10 minutos** e disponibilizar o link conforme o ambiente de submissão da FIAP.
