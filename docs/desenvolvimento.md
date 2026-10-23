# Desenvolvimento

## Requisitos

- Go 1.26+
- Docker com BuildKit (opcional)
- `staticcheck` (opcional): `go install honnef.co/go/tools/cmd/staticcheck@latest`

## Rodando

```sh
cp .env.example .env          # preencha PRAZO_ICAL_URL (entre aspas simples)
go run ./cmd/prazo keygen     # cole em PRAZO_MASTER_KEY
set -a; . ./.env; set +a

go run ./cmd/prazo doctor                       # diagnóstico do iCal + web services
go run ./cmd/prazo doctor -user SEU_USUARIO     # + funções liberadas e disciplinas (pede a senha)
go run ./cmd/prazo sync --dry-run               # próximos 30 dias, normalizados
go run ./cmd/prazo sync --dry-run -days 60 -links
go run ./cmd/prazo doctor -ics testdata/moodle_cct_anon.ics -skip-ws   # offline
go run ./cmd/prazo auth                         # autoriza o Google (uma vez)
go run ./cmd/prazo sync                         # aplica na agenda
```

## Google Agenda

1. No [Google Cloud Console](https://console.cloud.google.com/), crie um
   projeto e ative a **Google Calendar API**.
2. Em "Tela de consentimento OAuth", escolha **Externo** e adicione o seu
   e-mail em **Usuários de teste**.
3. Em "Credenciais → Criar credenciais → ID do cliente OAuth", escolha
   **App para computador** e baixe o JSON para `secrets/google-oauth.json`
   (`chmod 600`).
4. Para o token não expirar a cada 7 dias, clique em **Publicar app** (modo
   "Em produção"). Para uso pessoal não precisa de verificação; no login,
   aparece "Google não verificou este app" → "Avançado" → "Acessar Prazo".
5. `prazo auth`: abra a URL exibida, autorize e volte ao terminal.
6. `prazo sync`: cria a agenda **"Prazo — UDESC"** e envia os prazos. Pode
   repetir à vontade: nada é duplicado.

O token fica cifrado em `PRAZO_DATA_DIR`. Rodando local e no Docker, são dois
lugares diferentes (`./data` e o volume `prazo-data`): autorize em cada um.

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `PRAZO_ICAL_URL` | (vazio) | URL de exportação do calendário. **Segredo.** |
| `PRAZO_MOODLE_URL` | host do iCal ou `https://moodle.joinville.udesc.br` | base do Moodle |
| `PRAZO_MOODLE_TOKEN` | (vazio) | token de web service. **Segredo.** |
| `PRAZO_GOOGLE_CREDENTIALS` | (vazio) | caminho do JSON da credencial OAuth "App para computador" |
| `PRAZO_MASTER_KEY` | (vazio) | 32 bytes em base64 (`prazo keygen`); cifra os segredos em disco. **Segredo.** |
| `PRAZO_DATA_DIR` | `data` | token cifrado e estado (`/data` no Docker) |
| `PRAZO_CALENDAR_NAME` | `Prazo — UDESC` | nome da agenda criada no Google |
| `PRAZO_SYNC_SINCE` | (vazio) | `AAAA-MM-DD`: ignora prazos anteriores; vazio = tudo o que o Moodle exporta |
| `PRAZO_IGNORE_COURSES` | (vazio) | siglas ou shortnames a ignorar, separados por vírgula |
| `PRAZO_IGNORE_TYPES` | (vazio) | `assign`, `quiz`, `forum`, `exam`, `course_event`, `other` |
| `PRAZO_IGNORE_TITLE_REGEX` | (vazio) | regex RE2 aplicada ao título, ex.: `(?i)opcional\|extra` |
| `PRAZO_INCLUDE_OPENINGS` | `false` | manter o evento "Início de X" junto do Término |
| `PRAZO_TIMEZONE` | `America/Sao_Paulo` | fuso da exibição e dos eventos na agenda |
| `PRAZO_LOG_FORMAT` | `text` | `text` (dev) ou `json` (produção) |
| `PRAZO_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Uma configuração inválida lista todos os problemas de uma vez e encerra com
código 2.

## Qualidade

```sh
gofmt -l .
go vet ./...
staticcheck ./...
go test -race ./...
```

Todos passam limpos no estado atual. O `staticcheck` precisa ter sido
compilado com Go 1.26: `go run honnef.co/go/tools/cmd/staticcheck@2026.1 ./...`.

### Estratégia de testes

| Camada | Como é testada |
|---|---|
| `domain`, `moodle`, `collections` | tabelas de casos; funções puras |
| `source/ical` | fixtures `.ics` (sintética e real anonimizada) e `httptest.NewTLSServer` |
| `source/webservice`, `doctor` | `httptest.Server` imitando as respostas do Moodle |
| `httpx` | servidor que falha N vezes, limite de corpo, vazamento de token, Retry-After |
| `sync` | fake de `source.Source`, `calendartest.Fake`, KV em memória e relógio injetado |
| `calendar` | regras puras: id válido para o Google, cor estável, janela, lembretes |
| `calendar/google` | `httptest.Server` imitando a Calendar API (409, 404, 403 de cota, 429) e o endpoint de token (PKCE, state, escopo) |
| `crypto`, `store/filekv` | ida e volta, adulteração, chave errada, permissão 0600, segredo nunca em texto plano |
| `config` | `getenv` injetado (sem `os.Setenv`) |
| `cmd/prazo` | formatação da tabela e roteamento de comandos |

Testes que dependem de fuso importam `time/tzdata`, para não depender do
sistema (a imagem Alpine de build não tem tzdata).

## Docker

```sh
docker build --target test .     # go vet + go test dentro do build
docker compose build
setfacl -m u:65532:r secrets/google-oauth.json   # o container (UID 65532) precisa ler a credencial
docker compose run --rm -p 127.0.0.1:8085:8085 prazo auth -addr 0.0.0.0:8085
docker compose run --rm prazo    # sync
docker compose run --rm prazo sync --dry-run
```

O Dockerfile tem estas etapas:

| Etapa | O que faz |
|---|---|
| `deps` | `go mod download`, com cache em `/go/pkg/mod`; só refaz quando `go.mod`/`go.sum` mudam |
| `build` | compila com cache em `/root/.cache/go-build` (rebuild incremental) |
| `test` | `go vet` + `go test` |
| `final` | distroless, não-root, ~19 MB |

## Convenções de código

- **Sem frameworks nem ORM.** Stdlib sempre que basta.
- **Tipos e erros por pacote** em `types.go` e `errors.go`.
- **Construtores** (`NewClient`, `NewSource`, `NewService`) validam a entrada e
  escondem os campos internos.
- **Interfaces** só com mais de uma implementação real ou para um fake de teste.
- **Erros** com contexto (`%w`). Sentinelas para o chamador decidir, tipos
  quando o erro carrega dados.
- **Comentários** explicam o *porquê*: uma restrição do Moodle, um
  comportamento não óbvio. Não repetem o que o código já diz.
- **Commits** pequenos, em português, no padrão Conventional Commits.
