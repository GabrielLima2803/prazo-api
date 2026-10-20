# Desenvolvimento

## Requisitos

- Go 1.25+
- Docker com BuildKit (opcional)
- `staticcheck` (opcional): `go install honnef.co/go/tools/cmd/staticcheck@latest`

## Rodando

```sh
cp .env.example .env          # preencha PRAZO_ICAL_URL
set -a; . ./.env; set +a

go run ./cmd/prazo doctor                       # diagnóstico do iCal + web services
go run ./cmd/prazo doctor -user SEU_USUARIO     # + funções liberadas e disciplinas (pede a senha)
go run ./cmd/prazo sync --dry-run               # próximos 30 dias, normalizados
go run ./cmd/prazo sync --dry-run -days 60 -links
go run ./cmd/prazo doctor -ics testdata/moodle_cct_anon.ics -skip-ws   # offline
```

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `PRAZO_ICAL_URL` | (vazio) | URL de exportação do calendário. **Segredo.** |
| `PRAZO_MOODLE_URL` | host do iCal ou `https://moodle.joinville.udesc.br` | base do Moodle |
| `PRAZO_MOODLE_TOKEN` | (vazio) | token de web service. **Segredo.** |
| `PRAZO_IGNORE_COURSES` | (vazio) | siglas ou shortnames a ignorar, separados por vírgula |
| `PRAZO_IGNORE_TYPES` | (vazio) | `assign`, `quiz`, `forum`, `exam`, `course_event`, `other` |
| `PRAZO_IGNORE_TITLE_REGEX` | (vazio) | regex RE2 aplicada ao título, ex.: `(?i)opcional\|extra` |
| `PRAZO_INCLUDE_OPENINGS` | `false` | manter o evento "Início de X" junto do Término |
| `PRAZO_TIMEZONE` | `America/Sao_Paulo` | fuso usado só para exibição |
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

Todos passam limpos no estado atual.

### Estratégia de testes

| Camada | Como é testada |
|---|---|
| `domain`, `moodle`, `collections` | tabelas de casos; funções puras |
| `source/ical` | fixtures `.ics` (sintética e real anonimizada) e `httptest.NewTLSServer` |
| `source/webservice`, `doctor` | `httptest.Server` imitando as respostas do Moodle |
| `httpx` | servidor que falha N vezes, limite de corpo, vazamento de token, Retry-After |
| `sync` | fake de `source.Source` e relógio injetado |
| `config` | `getenv` injetado (sem `os.Setenv`) |
| `cmd/prazo` | formatação da tabela e roteamento de comandos |

Testes que dependem de fuso importam `time/tzdata`, para não depender do
sistema (a imagem Alpine de build não tem tzdata).

## Docker

```sh
docker build --target test .     # go vet + go test dentro do build
docker compose build
docker compose run --rm prazo    # sync --dry-run com o .env
```

O Dockerfile tem estas etapas:

| Etapa | O que faz |
|---|---|
| `deps` | `go mod download`, com cache em `/go/pkg/mod`; só refaz quando `go.mod`/`go.sum` mudam |
| `build` | compila com cache em `/root/.cache/go-build` (rebuild incremental) |
| `test` | `go vet` + `go test` |
| `final` | distroless, não-root, ~9 MB |

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
