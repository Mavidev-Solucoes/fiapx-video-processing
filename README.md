# FIAP X — Processamento de Vídeos (Hackathon Fase 5)

Projeto do Hackathon FIAP X. O objetivo é receber vídeos de usuários autenticados, processá-los em paralelo, gerar um ZIP com imagens extraídas, permitir download e acompanhar os estados dos trabalhos, sem perder solicitações aceitas em picos de demanda.

> **Estado: fundação técnica (PR inicial).** Esta versão contém a estrutura, infraestrutura local, pipeline de CI, contrato de dados inicial e o módulo de extração FFmpeg/ZIP com testes. **Não implementa ainda** login, upload, fila de jobs, execução por worker, status por usuário, armazenamento de vídeos/ZIPs nem notificações. O endpoint da API neste momento é apenas um health check.

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
cmd/api/                 servidor HTTP (somente health check por enquanto)
cmd/worker/              processo separado (consumo de fila pendente)
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

1. Registro/login seguro, emissão de token, autorização por usuário e API de jobs.
2. Upload com tamanho limitado, persistência de entrada e saída no MinIO e publicação segura de jobs no RabbitMQ.
3. Worker funcional, recuperação de solicitações, retries, atualização persistente de status, ZIP e notificações de falha.
4. Testes de concorrência/picos, integração, documentação final e demonstração de até 10 minutos.

Confira [decisões de arquitetura](docs/architecture.md) e [matriz de requisitos](docs/requirements.md).
