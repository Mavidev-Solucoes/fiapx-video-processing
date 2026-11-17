# FIAP X — Hackathon de processamento de vídeos

Aplicação para receber vídeos de usuários autenticados, processá-los de forma assíncrona e paralela com FFmpeg e disponibilizar um ZIP de frames. Baseada no protótipo Go entregue pela FIAP, mas com recepção, processamento e persistência desacoplados.

## Visão geral

- **API Go (net/http):** registro/login, autorização JWT, upload (100 MiB), listagem/status, download ZIP e notificações.
- **Workers Go + FFmpeg:** executam os jobs consumindo RabbitMQ; escalam por réplicas do Compose.
- **PostgreSQL:** usuários, jobs, estado, tentativas, outbox transacional, leases e notificações.
- **RabbitMQ:** fila durável `fiapx.jobs` com mensagens persistentes, publisher confirms e ACK manual.
- **Armazenamento S3 compatível (MinIO/Silo no Compose):** vídeos originais e ZIPs privados.
- **Docker Compose:** infraestrutura e execução local reproduzíveis, sem Kubernetes obrigatório.

```text
Navegador / cliente REST
        |
        v
Go API --------- PostgreSQL (jobs, outbox, notificações)
  |  |                   ^
  |  +--> S3 (vídeos)    |
  v                      |
RabbitMQ (fila durável)  |
  |                      |
  +--> Worker Go/FFmpeg --+--> S3 (ZIP)
  +--> Worker Go/FFmpeg
```

## Começar rapidamente

Pré-requisito: Docker com Compose v2.

```bash
git clone https://github.com/Mavidev-Solucoes/fiapx-video-processing.git
cd fiapx-video-processing
bash scripts/deploy-local.sh
```

Abra **http://localhost:8080/** e crie uma conta. A interface permite enviar um vídeo, acompanhar `queued → processing → completed/failed`, baixar o ZIP e visualizar notificações de falha. Os avisos são atualizados automaticamente a cada 4 segundos e podem ser marcados como lidos. A sessão do navegador fica apenas na memória; atualizar a página exige novo login.

Por padrão, o script executa **dois workers**. Para mudar: `WORKERS=3 bash scripts/deploy-local.sh`. Para desligar sem apagar os dados: `docker compose down`.

Observação: o ZIP contém **uma imagem PNG por segundo** (`ffmpeg -vf fps=1`), preservando o comportamento do protótipo FIAP; não contém todos os frames originais.

### Serviços auxiliares

| Serviço | Endereço local |
| --- | --- |
| UI/API | http://localhost:8080/ |
| RabbitMQ Management | http://localhost:15672/ |
| Console de objetos | http://localhost:9001/ |
| PostgreSQL | localhost:5432 |

**Segurança:** o Compose contém credenciais de desenvolvimento e expõe portas somente em `127.0.0.1`. Configure `AUTH_JWT_SECRET`, `POSTGRES_PASSWORD`, `RABBITMQ_PASSWORD`, `MINIO_USER` e `MINIO_PASSWORD` antes de qualquer implantação fora de um ambiente local controlado. Não exponha o Compose diretamente à internet sem TLS e revisão de segurança.

## Endpoints REST

| Método | Endpoint | Autorização | Finalidade |
| --- | --- | --- | --- |
| POST | `/api/auth/register` | Pública | Criar usuário, receber token |
| POST | `/api/auth/login` | Pública | Autenticar, receber token |
| GET | `/api/me` | Bearer | Consultar usuário |
| POST | `/api/jobs` | Bearer | Upload `multipart/form-data` (`video`); retorna HTTP 202 |
| GET | `/api/jobs` | Bearer | Listar os últimos 100 jobs próprios |
| GET | `/api/jobs/{id}` | Bearer | Consultar status do job próprio |
| GET | `/api/jobs/{id}/download` | Bearer | Baixar ZIP concluído; 409 se pendente |
| GET | `/api/notifications` | Bearer | Listar notificações próprias |
| POST | `/api/notifications/{id}/read` | Bearer | Marcar aviso como lido; retorna 204 |
| GET | `/health/live` | Pública | Health check da API |

Upload com extensões `.mp4`, `.mov`, `.mkv`, `.webm` e `.avi`; limite de **100 MiB**. O conteúdo é efetivamente validado pelo FFmpeg durante o processamento; extensões válidas não garantem vídeo decodificável.

Exemplo usando token da resposta de cadastro/login:

```bash
curl -i -X POST http://localhost:8080/api/jobs \
  -H "Authorization: Bearer TOKEN" \
  -F "video=@./exemplo.mp4"
```

Um segundo usuário não consegue consultar ou baixar jobs e notificações de outra conta (404 ou lista vazia).

## Por que as solicitações aceitas são recuperáveis?

1. O vídeo é salvo no armazenamento de objetos, depois o job e sua entrada de outbox são gravados **na mesma transação PostgreSQL**.
2. O HTTP **202** só é devolvido após o commit. Se RabbitMQ estiver indisponível, a entrada de outbox permanece pendente para publicação posterior.
3. O dispatcher usa `FOR UPDATE SKIP LOCKED`, fila durável, mensagens persistentes e confirmação do broker.
4. Os workers usam `ACK manual`, `prefetch=1`, lease exclusiva com heartbeat e token de fencing. Falhas transitórias são reenfileiradas (até três tentativas); entregas repetidas não reiniciam jobs concluídos.
5. Ao falhar definitivamente, o worker grava `failed` **e a notificação do dono** na mesma transação. A chave única impede avisos duplicados.

**Limites:** a demonstração usa volumes locais persistentes do Docker. Remover os volumes (por exemplo, `docker compose down -v`), perder permanentemente o armazenamento ou hospedar volumes em disco sem redundância não oferece recuperação de desastre. Notificações são **in-app**, não por e-mail. Arquivos órfãos de uploads não confirmados ainda podem exigir limpeza operacional.

## Testes e evidências

```bash
go test -race ./...
go vet ./...
bash scripts/smoke-intake.sh
bash scripts/smoke-processing.sh
```

O GitHub Actions executa testes unitários, análise estática, build Go, verificação do JavaScript, builds das imagens e testes end-to-end com Docker Compose. Os cenários verificam aceitação com RabbitMQ fora do ar, dois workers processando vídeos reais, PNGs no ZIP, isolamento entre usuários, idempotência, vídeo inválido, notificação privada e recuperação de lease expirada. Os scripts E2E **apagam os volumes de teste** ao terminar; não os execute sobre dados importantes.

## CI/CD e deployment

- **CI:** `.github/workflows/ci.yml` — obrigatório antes da publicação.
- **CD:** `.github/workflows/cd.yml` — após um CI verde de um `push` em `main`, publica imagens API e worker no GitHub Container Registry, com tags `latest` e SHA de commit. A publicação depende das permissões GHCR da organização; valide o primeiro run após o merge.
- **Deployment local a partir do código:** `bash scripts/deploy-local.sh`.
- **Deployment com imagens publicadas:** `FIAPX_TAG=<SHA> bash scripts/deploy-release.sh` (autentique-se com `docker login ghcr.io` caso as imagens não estejam públicas).

O deployment por Compose é explícito/reproduzível; não há deploy automático em um servidor remoto nem configuração AWS exigida para esta entrega. As migrations ficam em `migrations/` e são aplicadas em volumes novos e pelo `schema-migrate` em volumes existentes.

## Documentação da entrega

- [Arquitetura e justificativas](docs/architecture.md)
- [Matriz de requisitos FIAP](docs/requirements.md)
- [Roteiro da demonstração de até 10 minutos](docs/demo.md)
- [Scripts de deployment e criação de recursos](scripts)

Projeto da organização Mavidev Soluções para o Hackathon Fase 5 da Pós-Tech FIAP.
