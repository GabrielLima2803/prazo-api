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

### Fase 2: Google Agenda 🔄

Código pronto e testado com fakes; falta a primeira execução com a conta real.

- `internal/crypto`: AES-256-GCM com versão, nonce aleatório e AAD.
- `internal/store` + `filekv`: segredos em arquivos cifrados até o SQLite.
- `prazo keygen`: gera a `PRAZO_MASTER_KEY`.
- `prazo auth`: OAuth loopback com PKCE e `state`, escopo
  `calendar.app.created`, verificação do refresh token e do escopo concedido.
- `internal/calendar`: `Deadline → Event` neutro, com id determinístico, cor
  por disciplina, lembretes por tipo, horário real ou 30 min terminando no
  prazo, e `Busy` só para provas com intervalo.
- `internal/calendar/google`: `Sink` com upsert (insert → 409 → update),
  recriação da agenda apagada, retry em 429/5xx/403 de cota e `ErrAuth`.
- `prazo sync`: semestre inteiro, sem apagar nada; falhas isoladas não param
  a execução.
- Docker em Go 1.26; credencial via `secret` do compose.

Com dados reais: 55 eventos → 41 prazos (40 do semestre 2026/2 e 1 evento
pessoal de abril).

### App móvel e API 🔄

- `prazo serve`: API REST (ver [api.md](api.md)), sincronização periódica com
  jitter, uma execução por vez, graceful shutdown e o endereço certo para o
  celular impresso ao subir.
- `internal/prefs`: disciplinas e tipos ocultos, cor por disciplina e
  lembretes por tipo, aplicados tanto no app quanto no sync com o Google.
- [prazo-app](https://github.com/GabrielLima2803/prazo-app): Expo SDK 58 +
  TypeScript, Expo Router, React Query, tema claro/escuro, modo demonstração.
  Testado ponta a ponta contra o servidor com os dados reais.

## Próximos passos

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

### Fase 7: App móvel

O usuário quer um app com mais personalização. Arquitetura recomendada: o
servidor Go continua fazendo a sincronização (em segundo plano, mesmo com o
celular desligado) e o app é a interface.

- API HTTP no `prazo serve`: preferências (filtros, cores, lembretes por
  tipo, horário do evento), status da última sincronização, "sincronizar agora".
- No mesmo projeto do Google Cloud: credencial **"Aplicativo da Web"** para o
  servidor trocar o código do login, e **Android/iOS** para o login no app.
- Multiusuário: `user_id` em todas as tabelas, criptografia por usuário e
  limite de sincronizações concorrentes.
- Login do Moodle pelo app (token via `launch.php`, como o app oficial).

## Pontos em aberto

| # | Questão | Situação |
|---|---|---|
| 1 | Horizonte inicial da agenda | ✅ tudo o que o Moodle exporta (D13) |
| 2 | Evento de prazo (duração zero) no Google | ✅ horário real ou 30 min terminando no prazo (D12) |
| 3 | Rótulo do `groupselect` | hoje é "Atividade"; talvez "Grupo" |
| 4 | Atividades com o mesmo nome na mesma disciplina | resolvido na Fase 5 com o id do módulo |
| 5 | Evento pessoal antigo (abril) entra na agenda | usar `PRAZO_SYNC_SINCE` ou um filtro de eventos pessoais |
| 6 | Stack do app móvel | ✅ Expo + React Native (D16) |
| 7 | Acesso fora da rede local | HTTPS via Tailscale ou proxy com TLS |
| 8 | Publicar o app (APK/TestFlight) | EAS Build; exige liberar HTTP ou ter HTTPS |
