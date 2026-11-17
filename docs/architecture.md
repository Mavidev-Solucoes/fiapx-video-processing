# FIAP X — Proposta arquitetural (Fase 5)

## Contexto e decisão

O código-base fornecido pela FIAP tem uma API Go/Gin com um único `main.go` que recebe o vídeo, executa FFmpeg na mesma requisição HTTP e grava o ZIP no disco local. Essa abordagem não oferece isolamento por usuário, fila de trabalho nem recuperação de solicitações. A Fase 5 pede processamento simultâneo, persistência, autenticação, status por usuário, notificação em falhas e escalabilidade.

**Decisão:** dois processos implantáveis, **API** e **worker**, em um só repositório Go. Escolhemos Docker Compose para a entrega, permitido explicitamente pelo edital. Não há necessidade de Saga, banco NoSQL ou Kubernetes obrigatório.

```text
Usuário autenticado
       |
       v
API HTTP --- PostgreSQL (usuários, jobs e status)
       |  \
       |   +------ MinIO (vídeos e ZIPs)
       v
RabbitMQ (fila durável de jobs)
       |
       v
Worker(s) Go / FFmpeg -----> MinIO (ZIP final)
       |
       +--------------------> PostgreSQL (status)
       |
       +-- falha persistida --> notificação ao usuário (a implementar)
```

## Responsabilidades

- **API:** criar usuários e sessões, autorizar acesso somente aos próprios jobs, receber uploads com limite de tamanho, persistir um job e seu arquivo, enfileirar trabalho, consultar status e servir downloads autorizados. Registro, login, perfil e leitura restrita de jobs foram implementados; o upload, a publicação de trabalhos, o download e o consumidor de jobs ainda serão implementados.
- **Worker:** receber um ID de job (não o arquivo inteiro), recuperar o vídeo do MinIO, executar FFmpeg e criar ZIP, enviar o ZIP ao MinIO e atualizar o estado no PostgreSQL. Vários workers poderão consumir em paralelo.
- **PostgreSQL:** fonte de verdade dos jobs aceitos. A tabela `video_jobs` inclui ID, usuário proprietário, status, chaves de objetos e tentativas.
- **RabbitMQ:** separa o recebimento de vídeos do processamento intensivo em CPU. Mensagens deverão ser persistentes, com confirmação de publicação e ACK após persistir o resultado; retries e DLQ serão definidos na implementação.
- **MinIO:** dados binários fora do banco relacional e do filesystem efêmero das aplicações.

## Estados do job

`queued` → `processing` → `completed` ou `failed`. A API deve listar apenas os jobs do usuário autenticado, inclusive os que falharam. Em caso de falha, haverá um canal de aviso (a definir e implementar) além do status persistido.

## Recuperação de picos — requisito essencial

**Não considerar o requisito atendido nesta fundação.** Precisamos garantir que um trabalho aceito não se perca na falha entre registrar o job no PostgreSQL e publicá-lo no RabbitMQ. Para isso, a implementação deverá adotar um mecanismo de recuperação dos jobs `queued` que ainda não foram entregues (por exemplo, outbox transacional ou reconciliador) e consumidores idempotentes que suportem mensagens repetidas. O número de workers controla a concorrência. **Neste PR #2, a API autentica e consulta jobs, mas ainda não os aceita nem os enfileira.**

## Origem do processamento

O protótipo FIAP usa `ffmpeg -vf fps=1` e entrega PNGs em ZIP. A implementação inicial de `internal/processing` mantém esse contrato, substituindo nomes baseados em timestamp por diretórios temporários exclusivos. Ainda não está conectada à API nem aos workers.

## Decisões de escopo

- Um repositório com dois executáveis para reduzir custo de coordenação.
- Sem introduzir cache Redis sem necessidade observada.
- Não expor arquivos por rotas estáticas públicas; downloads exigirão propriedade de job.
- Docker Compose cobre entrega e demonstração, com teste de escalabilidade por múltiplos workers.
- Senhas de desenvolvimento do Compose não devem ser reutilizadas em ambientes públicos.

Veja [requisitos e critérios de aceite](requirements.md).

## Autenticação e autorização (PR #2)

A API usa `bcrypt` para armazenar senhas e tokens JWT HS256 com expiração de uma hora. O segredo é informado via `AUTH_JWT_SECRET` (mínimo 32 bytes). As consultas `GET /api/jobs` e `GET /api/jobs/{id}` filtram `user_id` no próprio SQL, impedindo acesso cruzado mesmo quando o ID de um job é conhecido. Registro e login são públicos; consulta de perfil e de jobs exige Bearer token.

Esta entrega não demonstra ainda upload, concorrência ou recuperação de pedidos; estes itens permanecem explicitamente pendentes.
