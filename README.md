# FIAP X — Hackathon de processamento de vídeos

Aplicação para receber vídeos de usuários autenticados, processá-los de forma assíncrona e paralela com FFmpeg e disponibilizar um ZIP de frames. Baseada no protótipo Go entregue pela FIAP, mas com recepção, processamento e persistência desacoplados.

## Visão geral

- **API Go (net/http):** registro/login, autorização JWT, upload (100 MiB), listagem/status, download ZIP e notificações.
- **Workers Go + FFmpeg:** executam os jobs consumindo RabbitMQ; escalam por réplicas do Compose.
- **PostgreSQL:** usuários, jobs, estado, tentativas, outbox transacional, leases e notificações.
- **RabbitMQ:** fila durável `fiapx.jobs` com mensagens persistentes, publisher confirms e ACK manual.
- **Armazenamento S3 compatível (MinIO/Silo no Compose):** vídeos originais e ZIPs privados.
- **Docker Compose:** infraestrutura e execução local reproduzíveis, sem Kubernetes obrigatório.
- **Prometheus + Grafana:** métricas operacionais da API e RabbitMQ, alertas avaliados pelo Prometheus e dashboard provisionado.

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

Abra **http://localhost:8080/** e crie uma conta. A interface permite enviar um vídeo, acompanhar `queued → processing → completed/failed`, baixar o ZIP e visualizar notificações de falha. Os avisos in-app são atualizados automaticamente a cada 4 segundos e podem ser marcados como lidos. Falhas terminais também geram e-mails quando SMTP está configurado. A sessão do navegador fica apenas na memória; atualizar a página exige novo login.

Por padrão, o script executa **dois workers**. Para mudar: `WORKERS=3 bash scripts/deploy-local.sh`. Para desligar a stack local sem apagar os dados: `docker compose -f docker-compose.yml -f docker-compose.mail.yml --profile monitoring down` (sem `-v`).

Observação: o ZIP contém **uma imagem PNG por segundo** (`ffmpeg -vf fps=1`), preservando o comportamento do protótipo FIAP; não contém todos os frames originais.

### Serviços auxiliares

| Serviço | Endereço local |
| --- | --- |
| UI/API | http://localhost:8080/ |
| RabbitMQ Management | http://localhost:15672/ |
| Mailpit (local email capture) | http://localhost:8025/ |
| Console de objetos | http://localhost:9001/ |
| PostgreSQL | localhost:5432 |
| Grafana (monitoramento) | http://localhost:3000/d/fiapx-operations |
| Prometheus (métricas e regras) | http://localhost:9090/ |

**Segurança:** o Compose contém credenciais de desenvolvimento e expõe portas somente em `127.0.0.1`. Configure `AUTH_JWT_SECRET`, `POSTGRES_PASSWORD`, `RABBITMQ_PASSWORD`, `MINIO_USER`, `MINIO_PASSWORD` e `GRAFANA_ADMIN_PASSWORD` antes de qualquer implantação fora de um ambiente local controlado. Não exponha o Compose diretamente à internet sem TLS e revisão de segurança.

## Monitoramento operacional

O script de deployment habilita o perfil Compose `monitoring` automaticamente. Abra **http://localhost:3000/d/fiapx-operations**. No ambiente local, use **admin** e senha **fiapx-monitoring-local** (ou as variáveis `GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD`). As configurações de datasource e dashboard são versionadas em `observability/`, portanto não exigem criação manual.

O Grafana apresenta **API disponível, exportador RabbitMQ disponível, número de workers consumidores, eventos de outbox pendentes, jobs por estado, profundidade da fila, tempo do job mais antigo em fila/processamento e notificações de erro não lidas**. A API expõe os agregados em `api:9091/metrics` apenas na rede Docker, sem dados de usuários. O RabbitMQ usa seu plugin oficial `rabbitmq_prometheus` e expõe filas em `rabbitmq:15692/metrics/detailed`. Ambos são coletados pelo Prometheus a cada 5 segundos. 

O arquivo `observability/prometheus/alerts.yml` possui regras para serviços indisponíveis, ausência de consumidores e acúmulo de jobs/outbox. Elas podem ser vistas no Prometheus em **Status → Rules / Alerts**. **Importante:** regras avaliadas não enviam páginas/e-mails automaticamente; um Alertmanager ou contact point seria necessário para notificação externa. Grafana e Prometheus são recursos locais de demonstração e mantêm dados em volumes Docker.

Para executar somente a aplicação sem observabilidade: `docker compose up -d --build --scale worker=2`. Para executar também o dashboard: `docker compose --profile monitoring up -d --build --scale worker=2`.

## Notificações de falha por e-mail (SMTP)

O FIAP X já possui registro/login, listagem de vídeos por usuário, downloads ZIP e avisos privados dentro da aplicação. Além disso, pode enviar **e-mails para o endereço cadastrado** quando um processamento termina em `failed`.

Na falha terminal, o worker grava o estado do job, o aviso in-app e a intenção de envio em `notification_email_outbox` **na mesma transação PostgreSQL**. Um dispatcher na API envia os e-mails e aplica retentativas com backoff; falhas SMTP não bloqueiam o processamento.

### Demonstração local com Mailpit (sem envio externo)

Execute `bash scripts/deploy-local.sh`. A stack disponibiliza o Mailpit em **http://localhost:8025/**. Cadastre um usuário de teste, envie um `.mp4` inválido e compare o status `failed` e o aviso in-app com a mensagem capturada no Mailpit.

O overlay `docker-compose.mail.yml` usa `SMTP_HOST=mailpit`, porta `1025`, e **TLS implícito com validação do certificado** gerado no ambiente local. As mensagens ficam no Mailpit; não são encaminhadas para caixas reais. O Compose base pode ser usado sem esse overlay.

### Configuração de um provedor SMTP real

Forneça `SMTP_HOST`, `SMTP_PORT` (padrão `465`), `SMTP_FROM` e, se necessário, `SMTP_USERNAME` e `SMTP_PASSWORD` por variáveis de ambiente ou `.env` não versionado. O transporte exige **TLS implícito com validação do certificado e hostname**; STARTTLS na porta 587 não é suportado. Um certificado de CA privado pode ser apontado por `SMTP_CA_FILE`. Não use o overlay Mailpit em produção.

Se `SMTP_HOST` estiver ausente, os avisos in-app continuam funcionando e as intenções de envio permanecem no banco. O envio tenta até oito vezes; sucesso significa **aceitação pelo relay SMTP**, não entrega garantida à caixa final. Uma queda entre aceite SMTP e commit pode gerar mensagem duplicada. Uma aplicação pública precisaria verificar o endereço de e-mail cadastrado e limitar abusos.

O script `bash scripts/smoke-email.sh` verifica entrega ao Mailpit, indisponibilidade temporária do servidor e recuperação sem duplicar intenções de envio.

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

**Limites:** a demonstração usa volumes locais persistentes do Docker. Remover os volumes (por exemplo, `docker compose down -v`), perder permanentemente o armazenamento ou hospedar volumes em disco sem redundância não oferece recuperação de desastre. O sistema oferece notificações **in-app** e envio **SMTP opcional**; na stack local, o Mailpit captura mensagens sem encaminhá-las para caixas de e-mail externas. Arquivos órfãos de uploads não confirmados ainda podem exigir limpeza operacional.

## Testes, cobertura e evidências

```bash
go test -race -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out # opcional: relatório navegável local
go vet ./...
bash scripts/smoke-intake.sh
bash scripts/smoke-processing.sh
bash scripts/smoke-monitoring.sh
bash scripts/smoke-email.sh
```

O job **Go build, vet and tests** do GitHub Actions executa os testes com detector de race e cobertura, publica o percentual agregado no resumo do job e salva `coverage.out` e `coverage-summary.txt` como artifact **`go-coverage-report`** (retenção de 30 dias). Para baixar: abra a execução do workflow e procure **Artifacts → go-coverage-report**. **Não existe percentual mínimo exigido nem bloqueio por threshold**: o perfil mede a cobertura dos testes Go e não substitui as provas E2E em contêineres.

Além disso, a CI executa build, `go vet`, validação do JavaScript, builds Docker e **quatro cenários de integração** (intake/RabbitMQ, FFmpeg/ZIP, Prometheus/Grafana e SMTP/Mailpit). As provas cobrem recuperação do broker, isolamento entre usuários, notificações e recuperação de processamento. Os scripts E2E **apagam volumes de teste** ao terminar; não os execute sobre dados importantes.

## CI/CD e deployment

- **CI/CD:** `.github/workflows/ci.yml` — testes e verificações obrigatórios; o job `publish` depende de **todos os seis jobs de validação** e roda somente em `push` para `main`, com `packages: write` concedido apenas a ele.
- **Publicação GHCR:** as imagens API e worker recebem tags `latest` e SHA do commit. Não há `workflow_run` privilegiado nem execução de código obtido de um workflow anterior. Verifique a publicação e os checks no CI da revisão `main` utilizada na entrega.
- **Deployment local a partir do código:** `bash scripts/deploy-local.sh`.
- **Deployment com imagens publicadas:** `FIAPX_TAG=<SHA> bash scripts/deploy-release.sh` (autentique-se com `docker login ghcr.io` caso as imagens não estejam públicas).

O deployment por Compose é explícito/reproduzível; não há deploy automático em um servidor remoto nem configuração AWS exigida para esta entrega. As migrations ficam em `migrations/` e são aplicadas em volumes novos e pelo `schema-migrate` em volumes existentes.

## Documentação da entrega

- [Arquitetura e justificativas](docs/architecture.md)
- [Matriz de requisitos FIAP](docs/requirements.md)
- [Roteiro da demonstração de até 10 minutos](docs/demo.md)
- [Scripts de deployment e criação de recursos](scripts)

Projeto da organização Mavidev Soluções para o Hackathon Fase 5 da Pós-Tech FIAP.
