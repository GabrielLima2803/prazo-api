# Arquitetura

## Visão geral

O fluxo de sincronização é `fetch → normalize → diff → apply`. Cada etapa
mora num pacote com uma responsabilidade só. O domínio não conhece Moodle,
Google nem banco.

```mermaid
flowchart LR
    subgraph Fontes
        ICAL[source/ical<br/>exportação iCal]
        WS[source/webservice<br/>API REST]
    end
    MOODLE[moodle<br/>convenções do Moodle<br/>puro, sem I/O]
    DOMAIN[domain<br/>Deadline, Normalize,<br/>Filter, título, hash]
    SYNC[sync<br/>caso de uso]
    CAL[calendar<br/>Deadline → Event<br/>interface Sink]
    GOOGLE[calendar/google<br/>Calendar API + OAuth]
    STORE[store<br/>interface KV]
    FILEKV[store/filekv<br/>arquivos cifrados<br/>SQLite na Fase 3]
    CRYPTO[crypto<br/>AES-256-GCM]

    ICAL -- usa --> MOODLE
    WS -- usa --> MOODLE
    MOODLE -- produz --> DOMAIN
    ICAL -- "[]domain.Deadline" --> SYNC
    SYNC --> DOMAIN
    SYNC --> CAL
    GOOGLE -. implementa .-> CAL
    SYNC --> STORE
    FILEKV -. implementa .-> STORE
    FILEKV --> CRYPTO
```

## Pacotes

```
cmd/prazo/                 composição: flags, config, dependências; sem regra de negócio
internal/
├── calendar/              Deadline → Event (id, cor, lembretes, janela) e a interface Sink
│   ├── google/            Sink com a Calendar API; OAuth loopback + PKCE
│   └── calendartest/      Sink em memória para testes
├── collections/           Set[T] e Counter[K] genéricos
├── config/                variáveis de ambiente → Config (valida tudo e reporta todos os erros)
├── crypto/                AES-256-GCM com versão e AAD (segredos em repouso)
├── domain/                entidades e regras puras
├── moodle/                interpretação de convenções do Moodle (nomes de evento, shortnames)
├── source/                interface Source
│   ├── ical/              cliente e parser da exportação iCal + adaptador para Source
│   └── webservice/        cliente da API REST e dos endpoints públicos do app
├── store/                 interface KV e chaves conhecidas
│   └── filekv/            KV em arquivos cifrados (0600, escrita atômica)
├── sync/                  caso de uso: Plan (ler + normalizar) e Sync (aplicar no destino)
├── doctor/                diagnóstico da instância (coleta separada da impressão)
├── httpx/                 HTTP com retry, backoff + jitter, limite de concorrência e de corpo
├── logging/               slog com redação de segredos
└── retry/                 backoff exponencial com full jitter (usado por httpx e google)
testdata/                  fixtures .ics (sintético + real anonimizado do CCT)
docs/                      esta documentação
```

### Organização interna de cada pacote

Todos seguem o mesmo padrão:

| Arquivo | Conteúdo |
|---|---|
| `types.go` | tipos exportados do pacote e a doc do pacote |
| `errors.go` | erros sentinela (`ErrX`) e tipos de erro (`*MoodleError`, `*StatusError`) |
| demais `.go` | comportamento, agrupado por assunto (`client.go`, `auth.go`, `api.go`, `parse.go`...) |
| `*_test.go` | testes da própria package (white-box) |

### Regras de dependência

- `domain` só importa a stdlib e `collections`.
- `moodle` importa `domain` (produz `DeadlineType`/`Phase`) e nunca faz I/O.
- `source/*` importam `domain` e `moodle`. Uma fonte nunca importa outra.
- `sync` depende das **interfaces** `source.Source`, `calendar.Sink` e
  `store.KV`, nunca das implementações.
- `calendar` não conhece o Google; `calendar/google` é a única parte que
  importa a API do Google.
- Só o `cmd/prazo` conhece todas as peças e as conecta.

Interfaces só existem onde há mais de uma implementação real ou um fake de
teste: `source.Source` (iCal e webservice, e o fake do `sync`),
`calendar.Sink` (Google e `calendartest`) e `store.KV` (arquivos agora,
SQLite na Fase 3).

## Tipos centrais

### `domain.Deadline`

```go
type Deadline struct {
    SourceID    string        // UID do iCal = id do evento no Moodle
    CourseCode  string        // sigla: POO
    CourseName  string        // shortname: 2026/2_CCT_CCI251-03A_POO0002
    Type        DeadlineType  // assign, quiz, forum, exam, course_event, other
    Phase       Phase         // due, open, close, grading, expect, ""
    Title       string        // nome da atividade sem o template do Moodle
    Description string
    URL         string
    StartsAt    time.Time     // sempre UTC
    EndsAt      time.Time     // sempre UTC
    Submitted   bool
    ContentHash string
}
```

O campo `Phase` não estava no desenho original. Entrou porque o Moodle cria
**vários eventos para a mesma atividade** (abertura, fechamento, conclusão
esperada), e o agrupamento precisa saber qual é qual.

Métodos:

- `DisplayTitle()` monta `[SIGLA] Tipo: Título` e adiciona `✅` quando a atividade foi entregue.
- `ComputeHash()` gera um SHA-256 dos campos de conteúdo, usado no diff.

### `domain.Normalize`

Função pura, aplicada nesta ordem:

1. **Agrupa** os eventos de uma mesma atividade. A chave é a disciplina mais o
   título normalizado, com um `map[string]Set[Phase]`.
   - Havendo Término, o Início é descartado (a menos que `IncludeOpenings`).
   - A "conclusão esperada" é descartada quando existe Término ou prazo de entrega.
   - Início e Término a até 12 h um do outro viram **um intervalo** com o
     horário real (ex.: prova online das 19h às 21h).
2. **Descarta** os prazos de correção, que só interessam a professores.
3. **Aplica os filtros** do usuário: disciplina, tipo e regex de título. A busca
   é O(1) com `Set`.
4. **Calcula** o `ContentHash`.
5. **Ordena** por data e `SourceID`, para a saída ser determinística.

### `source.Source`

```go
type Source interface {
    Fetch(ctx context.Context) ([]domain.Deadline, error)
}
```

Contrato: **um erro significa que a lista não está completa.** Quem chama nunca
pode apagar eventos com base num fetch que falhou. A Fase 3 implementa essa
proteção no diff.

### `sync.Service` e `sync.Plan`

`Service.Plan(ctx)` busca a fonte e normaliza o resultado, e devolve um `Plan`
com os prazos, a quantidade bruta recebida e o instante do cálculo. O relógio
é injetável (`WithClock`) para os testes.

`Plan.Upcoming(window)` recorta a janela usada na tabela do dry-run.

`Service.Sync(ctx, Target)` calcula o plano e aplica cada prazo no `Sink`:

- garante a agenda dedicada (recria se o usuário apagou) e guarda o id no `KV`;
- faz upsert de cada evento: o id é determinístico, então repetir não duplica;
- uma falha isolada não interrompe os demais; `calendar.ErrAuth`,
  `calendar.ErrCalendarGone` e cancelamento interrompem;
- **não apaga nada** até a Fase 3, que traz o estado necessário para
  distinguir "apagado no Moodle" de "saiu da janela da exportação".

### `calendar.Event`

`calendar.FromDeadline` traduz o prazo para a agenda:

| Aspecto | Regra |
|---|---|
| id | base32hex de `prazo:<SourceID>` (alfabeto `a-v0-9` do Google) |
| janela | intervalo real quando existe; senão 30 min **terminando** no prazo |
| cor | FNV da sigla → `colorId` 1–11, estável entre execuções |
| lembretes | entrega 3d/1d/2h · prova 7d/1d · questionário 1d/2h · demais 1d · nenhum se entregue |
| ocupado | só provas com intervalo real bloqueiam a agenda; prazos ficam "livre" |
| metadados | `extendedProperties.private`: `source_id`, `content_hash` |

## Tratamento de erros

- Os erros sempre carregam contexto: `fmt.Errorf("fetch ical: %w", err)`.
- Condições que o chamador precisa distinguir viram sentinelas, testáveis com
  `errors.Is`. Exemplos: `ical.ErrNotICalendar`, `webservice.ErrServiceUnavailable`
  e `httpx.ErrBodyTooLarge`.
- Erros que carregam dados são tipos, testáveis com `errors.As`:
  `httpx.StatusError` (código e Retry-After) e `webservice.MoodleError`
  (função, código e mensagem).
- Não há `panic` fora do `main`.

## Fuso horário

O domínio inteiro trabalha em UTC. A conversão para `America/Sao_Paulo`
(configurável em `PRAZO_TIMEZONE`) acontece só na borda: tabela, mensagens,
o fuso da agenda criada e o `timeZone` de cada evento.

O binário embute o `time/tzdata`, então funciona na imagem distroless.
