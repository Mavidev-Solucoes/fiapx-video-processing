# FIAP X — Processamento de Vídeos (Hackathon Fase 5)

Projeto do Hackathon FIAP X. O objetivo é receber vídeos de usuários autenticados, processá-los em paralelo, gerar um ZIP com imagens extraídas, permitir download e acompanhar os estados dos trabalhos, sem perder solicitações aceitas em picos de demanda.

> **Estado: fluxo funcional de vídeo (PR #4).** A API autentica e recebe vídeos, o outbox publica jobs duráveis, e workers independentes consomem RabbitMQ, extraem frames via FFmpeg, geram ZIP, atualizam o estado e permitem download privado. Notificações de falha e documentação final de entrega continuam no PR #5.

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
cmd/worker/              processo independente (consumer RabbitMQ + FFmpeg)
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
| `POST /api/jobs` | Sim | Upload multipart (`video`) e aceitação assíncrona (HTTP 202) |
| `GET /api/jobs/{id}` | Sim | Detalhe do job pertencente ao usuário; 404 para jobs de terceiros |
| `GET /api/jobs/{id}/download` | Sim | ZIP privado quando completed; 409 enquanto pendente |
| `GET /health/live` | Não | Liveness da API |

Exemplo de teste manual após `docker compose up --build -d`:

```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"estudante@example.com","password":"uma-senha-bem-forte-2026"}'
```

O resultado inclui `accessToken`. Envie o valor no cabeçalho `Authorization: Bearer <accessToken>` para acessar `/api/me` e `/api/jobs`. A lista retorna os vídeos já enviados pelo usuário, inicialmente com estado `queued` até a implementação do worker.

**Situação atual:** processamento por worker, ZIP e download privado implementados. A notificação de erros e as evidências finais ficam para o PR #5.

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
2. **Concluído neste PR:** upload com tamanho limitado, persistência no MinIO e publicação recuperável pelo outbox PostgreSQL/RabbitMQ.
3. **Concluído neste PR:** worker funcional, retries limitados, status persistidos, ZIP e downloads autorizados. **Notificações pendentes**.
4. Testes de concorrência/picos, integração, documentação final e demonstração de até 10 minutos.

Confira [decisões de arquitetura](docs/architecture.md) e [matriz de requisitos](docs/requirements.md).

## Upload de vídeo e despacho assíncrono (PR #3)

Envie um vídeo em `POST /api/jobs` com autenticação Bearer, usando `multipart/form-data` e o campo de arquivo **`video`**. Extensões permitidas: `.mp4`, `.mov`, `.mkv`, `.webm` e `.avi`. Limite de **100 MiB** por upload. O retorno `202 Accepted` inclui o job com estado `queued` e o cabeçalho `Location: /api/jobs/{id}`.

A API envia o arquivo ao MinIO (objeto privado por usuário/job). **Só considera o job aceito após inserir a linha de `video_jobs` e a entrada de `video_job_outbox` na mesma transação PostgreSQL.** O dispatcher lê as entradas pendentes com `FOR UPDATE SKIP LOCKED` e as publica na fila durável `fiapx.jobs`, usando mensagens persistentes e publisher confirms. Se RabbitMQ estiver indisponível, a entrada permanece pendente no banco e é tentada novamente. Uma queda após o publish e antes do commit pode gerar duplicidade; o worker deverá processar mensagens de forma idempotente no PR #4.

Para volumes PostgreSQL já criados nos PRs anteriores, o serviço `schema-migrate` aplica `migrations/002_job_outbox.sql` automaticamente ao subir o Compose. Em volumes novos, o arquivo também é aplicado pelo PostgreSQL na inicialização. O serviço `minio-init` configura o bucket antes de a API aceitar uploads.

Exemplo (substitua `TOKEN` por um token real):

```bash
curl -i -X POST http://localhost:8080/api/jobs \
  -H 'Authorization: Bearer TOKEN' \
  -F 'video=@./exemplo.mp4'
```

**Escopo atual:** o worker consome jobs, gera ZIP e registra completed/failed, com retries e idempotência. Não use `docker compose down -v` se quiser preservar dados locais. Não use `docker compose down -v` se quiser manter os dados locais.

## Worker, ZIP e download (PR #4)

O worker utiliza RabbitMQ com ACK manual e prefetch 1, permitindo paralelismo com o comando `docker compose up -d --build --scale worker=2`. Cada job é adquirido por atualização condicional no PostgreSQL: `queued → processing → completed/failed`. Uma `lease_token` (UUID) com prazo de validade impede execução simultânea de mensagens duplicadas. Durante a execução, o worker renova a lease a cada 20 segundos. Após um crash, a lease expira e permite retomar o job com outra entrega do RabbitMQ.

O vídeo é baixado do MinIO, processado por FFmpeg com `fps=1` (projeto-base), compactado como `frames.zip` e enviado ao MinIO. O PostgreSQL recebe a chave final somente após o arquivo ter sido persistido. Mensagens são confirmadas ao RabbitMQ somente depois do estado terminal. Falhas transitórias são reenfileiradas com até três tentativas; vídeos incompatíveis passam diretamente a `failed`.

O endpoint `GET /api/jobs/{id}/download` responde com `application/zip` apenas quando o token pertence ao usuário dono do job e o estado é `completed`. Jobs pendentes recebem HTTP 409; acesso a trabalhos de terceiros recebe 404.

### Como comprovar

- `go test -race ./...`: testes de auth, upload, FFmpeg/ZIP, workflow e proteção do download.
- `bash scripts/smoke-intake.sh`: aceitação durante interrupção de RabbitMQ e recuperação do outbox.
- `bash scripts/smoke-processing.sh`: duas réplicas de worker, upload de vídeos reais gerados pelo FFmpeg, ZIP com PNGs, isolamento entre usuários, supressão de duplicidades e erro de mídia inválida.

**Ainda pendente:** notificação de falhas ao usuário, revisão de resiliência e apresentação final (PR #5).
