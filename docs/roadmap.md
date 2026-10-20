# Roadmap

## Entregue

### Fase 0: Descoberta ✅

- `prazo doctor`:
  - analisa o iCal: contagens, formatos de UID, propriedades, disciplinas,
    regras de título, títulos não reconhecidos e próximos eventos;
  - consulta a configuração pública do Moodle e testa `/login/token.php` sem
    credencial real;
  - com token, lista as funções liberadas, as disciplinas (com sigla) e os
    eventos pendentes.
- Clientes base: `httpx` (retry, backoff e concorrência), `logging` (redação),
  parser iCal e cliente webservice.
- As regras foram calibradas com dados reais do Moodle do CCT. Ver
  [moodle-udesc.md](moodle-udesc.md).
- Docker em etapas com cache do BuildKit e compose endurecido.

### Fase 1: Leitura ✅

- `prazo sync --dry-run` lê o iCal, normaliza e imprime os próximos N dias.
- `domain.Normalize`: agrupamento Início/Término/conclusão, descarte de
  correção, filtros, hash e ordenação determinística.
- Título padronizado (`[POO] Entrega: Aula Prática 9`) e link da atividade
  (ou do dia no calendário).
- Filtros por variável de ambiente.

Resultado com dados reais: 54 eventos → 40 prazos, com 100% dos eventos de
disciplina reconhecidos.

### Refatoração estrutural

- `moodle` separado de `domain` (D2).
- `types.go`/`errors.go` por pacote. Construtores no lugar de structs com
  campos públicos.
- `collections.Set`/`Counter` substituem os mapas soltos.
- `doctor` com a coleta separada da impressão, e `Result[T]` para verificações
  que podem falhar.
- `config` aninhada, com todos os erros reportados de uma vez e `getenv`
  injetável.
- `sync.Service` com relógio injetável. `Plan` como tipo, no lugar de três
  valores de retorno.
- `cmd/prazo` com registro de comandos e um `app` com as dependências
  compartilhadas.

## Próximos passos

### Fase 2: Google Agenda (próxima)

Pré-requisito do usuário: criar um projeto no Google Cloud, ativar a Calendar
API e criar uma credencial OAuth do tipo **"App para computador"**.

1. **`internal/crypto`**: AES-256-GCM com `PRAZO_MASTER_KEY` (32 bytes em
   base64), nonce aleatório por registro, `Seal`/`Open`, testes de ida e volta
   e de adulteração.
2. **`prazo auth`**: fluxo OAuth por loopback (`127.0.0.1:<porta aleatória>`)
   com PKCE e `state`, escopo `calendar.app.created` (D8). O refresh token é
   salvo criptografado. Até o SQLite da Fase 3, num arquivo `0600` em `/data`.
3. **`internal/calendar`**:
   - interface `Sink` (`EnsureCalendar`, `Upsert`, `Delete`), uma implementação
     Google e uma fake para testes;
   - ID determinístico em base32hex derivado do `SourceID` (D9);
   - `extendedProperties.private`: `source_id` e `content_hash`;
   - cor estável por disciplina: hash da sigla → `colorId` 1–11;
   - lembretes por tipo: entrega → 3 dias, 1 dia e 2 h; prova → 7 dias e 1 dia;
   - sem lembretes quando `Submitted`;
   - fuso `America/Sao_Paulo` no evento;
   - prazos de duração zero viram eventos de 30 min que **terminam** no prazo;
   - backoff em 403 `rateLimitExceeded` e 429.
4. **`prazo sync`** (sem `--dry-run`): `EnsureCalendar("Prazo — UDESC")` e
   upsert de tudo. Ainda sem estado: o upsert idempotente garante que não
   duplica.
5. **Horizonte**: sincronizar só dos **últimos 7 dias em diante**. Ainda
   precisa da confirmação do usuário.

### Fase 3: Estado e diff

- `internal/store`:
  - interface de repositório e implementação SQLite (`modernc.org/sqlite`, sem CGO);
  - migrations versionadas em `migrations/`;
  - tabelas `deadlines` (`source_id`, `google_event_id`, `content_hash`,
    `deleted_at`), `sync_runs` e `secrets`.
- Diff em `sync`:
  - novo → create;
  - hash diferente → update;
  - igual → no-op;
  - sumiu → delete, **só** se o fetch foi completo;
  - **proteção contra apagar em massa**: se mais de X% (configurável, padrão
    30%) sumiu de uma vez, aborta e loga um alerta.
- Persistir o estado em transação, só depois de o Google confirmar.
- Registrar uma linha em `sync_runs` por execução.
- Testes:
  - idempotência: a segunda execução não faz nenhuma escrita;
  - resiliência: fonte com erro ou vazia não apaga nada;
  - apagar em massa é bloqueado.

### Fase 4: Serviço

- `prazo serve`:
  - ticker com jitter (padrão 30 min);
  - lock que impede duas sincronizações ao mesmo tempo;
  - graceful shutdown (`SIGTERM`).
- HTTP:
  - `/healthz` (o processo está vivo);
  - `/readyz` (o último sync deu certo dentro de 2 intervalos).
- Compose com `restart: unless-stopped`, healthcheck e comando `serve`.

### Fase 5: Web services e status de entrega

- `source/webservice` implementa `source.Source`, usando
  `core_calendar_get_action_events_by_timesort` mais
  `mod_assign_get_submission_status`.
- Fallback automático para o iCal quando o token falha ou o serviço está
  desabilitado, com uma mensagem clara.
- A chave de agrupamento passa a usar o id do módulo, que é exato (D3).
- `Submitted` → prefixo `✅` e remoção dos lembretes.
- Token via SSO (`launch.php`) para contas id.udesc/office365.

### Fase 6: Notificações

- `internal/notify` com Telegram (bot token + chat id).
- Resumo diário às 7h e semanal no domingo à noite, agrupado por dia.

### Fase 7: Multiusuário (opcional)

- Página web mínima para outros alunos conectarem Moodle e Google.
- `user_id` em todas as tabelas e criptografia por usuário.
- Limite de sincronizações concorrentes.

## Pontos em aberto

| # | Questão | Proposta |
|---|---|---|
| 1 | Horizonte inicial da agenda | últimos 7 dias em diante |
| 2 | Evento de prazo (duração zero) no Google | 30 min terminando no prazo, para aparecer na grade |
| 3 | Rótulo do `groupselect` | hoje é "Atividade"; talvez "Grupo" |
| 4 | Atividades com o mesmo nome na mesma disciplina | resolvido na Fase 5 com o id do módulo |
