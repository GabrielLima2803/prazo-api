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

### Fase 2: Google Agenda ✅

Validada com a conta real em 23/10/2026: 41 eventos criados na agenda
"Prazo — UDESC" sem falhas; a segunda execução fez 0 criações e 41
atualizações (idempotente).

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

### Multiusuário, diff e remoção segura ✅

- `internal/tenant`: cada aluno tem um espaço próprio no KV (`u<id>_*`):
  conta do Moodle, token do Google, preferências, estado da sincronização.
  O id é um hash do host + userid do Moodle. Limite `PRAZO_MAX_USERS`.
- Login aceito só nos Moodles de `PRAZO_MOODLE_HOSTS` (os da UDESC).
- Sessões do app ligadas a um usuário (até 10 aparelhos por pessoa).
- Sincronização de todos, um de cada vez, a cada `PRAZO_SYNC_INTERVAL`.
- Migração automática da instalação de um usuário só, sem duplicar a agenda.
- `sync` guarda o que está na agenda (`sync_state`): só envia o que mudou
  (com dados reais, 41 iguais em 0,5 s em vez de 41 chamadas em 30 s); apaga
  o que foi ocultado ou removido do Moodle; prazo antigo que saiu da
  exportação só é esquecido; mais de 30% sumindo de uma vez bloqueia as
  remoções.
- Status de entrega: um prazo vira ✅ (e perde os lembretes) quando já
  apareceu como pendente no Moodle e saiu da lista antes do prazo.
- Docker de produção: `prazo serve` com `prazo healthcheck`,
  `restart: unless-stopped`, dados e segredos montados do host.

## Próximos passos

### Publicar os builds do app

- Android: `npx eas-cli build -p android --profile preview` gera um APK
  (precisa de conta grátis no expo.dev). Alternativa: build local com o SDK
  do Android.
- iPhone: TestFlight exige a conta de desenvolvedor da Apple (US$ 99/ano);
  sem ela, Expo Go + EAS Update.
- Google Cloud: cada amigo precisa estar em "Usuários de teste" (até 100)
  enquanto o app do Google não for verificado.

### Depois

- SQLite no lugar dos arquivos cifrados (a interface `store.KV` já isola).
- Servidor sempre ligado (VPS ou Raspberry Pi) em vez do PC.
- Token de web service do Moodle via SSO (`launch.php`) para contas id.udesc.

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
