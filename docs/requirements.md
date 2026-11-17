# FIAP X — Matriz de aderência ao Hackathon Fase 5

Base: enunciado `POSTECH - SOAT - Fase 5 - Hacka.pdf`, páginas 3–4. Esta matriz resume os critérios técnicos implementados; o vídeo e sua disponibilização são atividades de entrega.

| Requisito | Implementação / critério verificável | Evidência |
| --- | --- | --- |
| Processamento simultâneo de vídeos | Dois ou mais containers worker consumindo em paralelo com `prefetch=1` | `smoke-processing.sh`, CI E2E |
| Não perder solicitação aceita durante indisponibilidade | Upload e outbox transacional persistidos; publicação com confirmação; lease e ACK manual no consumo | `smoke-intake.sh`, recuperação de lease no `smoke-processing.sh` |
| Autenticação com usuário e senha | Bcrypt e JWT; endpoints privados | Testes `internal/auth`, `internal/httpapi` |
| Status por usuário | `queued`, `processing`, `completed`, `failed` filtrados por proprietário | API `/api/jobs`; E2E com usuários A/B |
| Notificação em caso de erro | Aviso in-app e email SMTP com persistência atômica, retries e envio ao Mailpit local | `/api/notifications`, `smoke-processing.sh`, `smoke-email.sh` |
| Banco de dados e persistência | PostgreSQL com migrations, volumes; armazenamento privado S3 | `migrations/`, `docker-compose.yml` |
| Arquitetura escalável | API/worker separados, escaláveis por `--scale worker=N` | `cmd/api`, `cmd/worker`, scripts |
| Versionamento GitHub | Repositório com commits e PRs isolados | Histórico do GitHub |
| Testes e automação | Go race tests, build, vet, Docker Compose, quatro testes E2E | `.github/workflows/ci.yml` |
| CI/CD | Validação em PR e imagens GHCR após CI verde no main; deploy Compose reproduzível | `.github/workflows/ci.yml` (job `publish`), `scripts/deploy-release.sh` |
| Monitoramento (stack recomendada) | Prometheus + Grafana, métricas dos jobs e RabbitMQ, regras de alerta | `observability/`, `smoke-monitoring.sh`, dashboard provisionado |
| Documentação de arquitetura | Diagrama, decisões, fluxos e limitações | `docs/architecture.md` |
| Scripts de criação de recursos | Compose, migrations e scripts de deployment | `docker-compose.yml`, `migrations/`, `scripts/` |
| Vídeo de até 10 minutos | Demonstrar login, upload, processamento, ZIP, falha, aviso e evidência CI | Pendente: gravação e link da apresentação |

**Escopo da garantia de durabilidade:** as requisições já confirmadas são recuperáveis em falhas transitórias de serviço/broker se os volumes persistentes estiverem intactos. Isso não equivale a disaster recovery diante da destruição do armazenamento.

## Cenários mínimos antes da gravação

1. Criar conta A, enviar vídeo e demonstrar estados até `completed`, com ZIP contendo PNGs.
2. Escalar dois workers; enviar mais de um vídeo e observar processamento.
3. Simular indisponibilidade de RabbitMQ ou lease expirada e demonstrar recuperação.
4. Autenticar como usuário B para comprovar isolamento do ZIP e das notificações.
5. Enviar um `.mp4` inválido; mostrar `failed`, notificação e confirmação de leitura.
6. Mostrar pipeline CI aprovado, publicação de imagens após merge e scripts de deployment local.

Não afirmar que o vídeo foi entregue nem que a primeira publicação GHCR está validada antes de obter os respectivos links e runs.
