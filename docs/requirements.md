# FIAP X — Matriz de requisitos e critérios de aceite

Fonte: `POSTECH - SOAT - Fase 5 - Hacka.pdf`, páginas 3 e 4.

**Legenda:** *Planejado* = ainda não existe; *Fundação* = estrutura/configuração presente, integração pendente; *Parcial* = implementação sem fluxo de ponta a ponta.

| Requisito FIAP | Critério de aceite proposto | Estado após PR #2 |
| --- | --- | --- |
| Processar mais de um vídeo simultaneamente | Duas solicitações aceitas e concluídas com workers paralelos | Planejado |
| Não perder requisição em picos | Job aceito permanece em banco e volta à fila após falha/restart | Planejado |
| Usuário e senha | Login com senha armazenada por hash; token nas rotas privadas | Implementado: bcrypt + token HS256 (CI pendente de validação) |
| Status de vídeos por usuário | Listagem restrita ao dono, com estados e erros | Parcial: consultas protegidas implementadas; criação/atualização de jobs pendente |
| Usuário pode ser notificado em erro | Notificação visível ou outro canal demonstrável quando job falha | Planejado |
| Persistência | Banco e armazenamento persistentes, com migração reproduzível | Parcial: tabela e repositório PostgreSQL; arquivos no MinIO ainda não integrados |
| Arquitetura escalável | API desacoplada do worker; escalar réplicas dos workers | Fundação |
| GitHub | Código versionado em repositório | Fundação |
| Testes | Testes unitários e de integração executados no CI | Parcial: testes ZIP, token, autenticação e isolamento de usuários |
| CI/CD | Workflow de testes, build, imagens e entrega reproduzível | Fundação (CI; CD pendente) |
| Documentação arquitetural | Diagrama, decisões, fluxos, erros e como reproduzir | Fundação |
| Scripts de recursos | DDL de banco e Compose | Fundação |
| Vídeo de até 10 minutos | Mostrar docs, arquitetura e aplicação funcionando | Planejado |

## Cenários de demonstração recomendados

1. Registrar usuário A, autenticar, enviar vídeo, acompanhar `queued` → `processing` → `completed` e baixar ZIP.
2. Executar dois vídeos com workers paralelos; demonstrar que ambos finalizam.
3. Simular pico/falha do worker e demonstrar recuperação sem perder job aceito.
4. Mostrar que usuário B não vê nem baixa trabalhos de A.
5. Enviar vídeo inválido e demonstrar falha persistida e notificação.
6. Mostrar CI e comandos para reproduzir localmente.

Não declarar requisitos como concluídos até seus critérios serem comprovados.
