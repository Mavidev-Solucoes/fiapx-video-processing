# FIAP X — Processamento de Vídeos (Hackathon Fase 5)

Projeto do Hackathon FIAP X. O objetivo é receber vídeos de usuários autenticados, processá-los em paralelo, gerar um ZIP com imagens extraídas, permitir download e acompanhar os estados dos trabalhos, sem perder solicitações aceitas em picos de demanda.

> **Estado: implementação incremental (PR #2).** O projeto já possui registro/login com hash bcrypt, tokens de acesso assinados, consultas de perfil e consulta persistida de jobs **restritas ao usuário autenticado**. Ainda não há upload, publicação de jobs no RabbitMQ, consumo pelo worker, download, notificação de falhas ou CD completo.

## Componentes previstos

- **API (Go):** autenticação, criação de jobs, consultas por usuário e downloads autorizados.
- **Worker (Go + FFmpeg):** consumidor RabbitMQ que processará jobs independentemente da API.
- **PostgreSQL:** usuários, metadados dos vídeos e estados dos jobs.
- **RabbitMQ:** fila durável para absorver picos e distribuir trabalho.
- **MinIO:** armazenamento de arquivos de entrada e resultados (compatível com S3).
- **Docker Compose:** execução local; não requer Kubernetes para atender ao escopo do Hackathon.

API e worker são aplicações distintas no mesmo repositório. O código fornecido pela FIAP era um protótipo Go/Gin com processamento síncrono em `main.go`, FFmpeg com `fps=1` e ZIP local. Preservamos o **comportamento de extração de um quadro por segundo** no módulo `internal/processing`; as rotas públicas do protótipo não foram copiadas por não atenderem à autenticação e ao isolamento por usuário.

## Estrutura

```text
cmd/api/                 servidor HTTP (autenticação e consultas de jobs)
cmd/worker/              processo separado (consumo de fila pendente)
internal/auth/           bcrypt + tokens HMAC-SHA256 e testes
internal/httpapi/        rotas REST, autorização e testes de handlers
internal/store/          persistência PostgreSQL (usuários + jobs)
internal/identity/       criação/validação de IDs UUID
internal/processing/     extração FFmpeg + criação de ZIP e testes
migrations/              DDL inicial de usuários e jobs
deploy/                  Dockerfiles separados
docs/                    arquitetura e rastreabilidade dos requisitos
.github/workflows/       pipeline de CI
docker-compose.yml       postgres, rabbitmq, minio, api e worker
```

## Executar a fundação localmente

Pré-requisitos: Docker com Compose v2. Para os testes sem Docker, Go 1.23+.

```bash
docker compose up --build -d
curl http://localhost:8080/health/live
docker compose ps
docker compose down
```

- API: http://localhost:8080/health/live
- RabbitMQ Management: http://localhost:15672 (credenciais de desenvolvimento no Compose)
- MinIO Console: http://localhost:9001 (credenciais de desenvolvimento no Compose)
- PostgreSQL: porta 5432, banco `fiapx` (schema inicial criado ao iniciar volume vazio)

**Atenção:** credenciais e portas do Compose são apenas para desenvolvimento local, nunca produção. A migração em `migrations/` roda na primeira inicialização de um volume PostgreSQL vazio; para aplicar alterações posteriores, será necessário um mecanismo de migrações.

## Autenticação e consulta de jobs

A API utiliza **hash bcrypt** para as senhas (12–72 bytes) e **Bearer JWT HS256**, válido por uma hora. A chave `AUTH_JWT_SECRET` é obrigatória na aplicação (mínimo 32 bytes); o valor padrão no Docker Compose é **somente para desenvolvimento local** e deve ser substituído fora desse ambiente.

| Método e rota | Autenticado? | Resultado |
| --- | --- | --- |
| `POST /api/auth/register` | Não | Cria conta e devolve token |
| `POST /api/auth/login` | Não | Autentica e devolve token |
| `GET /api/me` | Sim | Perfil do usuário |
| `GET /api/jobs` | Sim | Lista até 100 jobs pertencentes ao usuário |
| `GET /api/jobs/{id}` | Sim | Detalhe do job pertencente ao usuário; 404 para jobs de terceiros |
| `GET /health/live` | Não | Liveness da API |

Exemplo de teste manual após `docker compose up --build -d`:

```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"estudante@example.com","password":"uma-senha-bem-forte-2026"}'
```

O resultado inclui `accessToken`. Envie o valor no cabeçalho `Authorization: Bearer <accessToken>` para acessar `/api/me` e `/api/jobs`. A lista estará vazia até o fluxo de upload e processamento ser implementado.

**Limitações atuais:** não há criação de jobs por API, processamento assíncrono, upload ou download neste PR. O banco PostgreSQL deve estar disponível para a API iniciar.

## Testes e CI

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/api ./cmd/worker
docker compose config --quiet
```

O workflow de GitHub Actions executa build, vet, testes/race e valida os Dockerfiles e o Compose. Isso valida a **fundação**, não o atendimento funcional completo da Fase 5.

## Próximas entregas (PRs separados)

1. **Concluído neste PR:** registro/login, token assinado, autorização e leitura de jobs por usuário.
2. **Próximo PR:** upload com tamanho limitado, persistência de entrada no MinIO e publicação recuperável de jobs no RabbitMQ.
3. Worker funcional, retries, atualização persistente de status, armazenamento do ZIP, downloads autorizados e notificações de falha.
4. Testes de concorrência/picos, integração, documentação final e demonstração de até 10 minutos.

Confira [decisões de arquitetura](docs/architecture.md) e [matriz de requisitos](docs/requirements.md).
