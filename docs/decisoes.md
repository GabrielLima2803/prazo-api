# Registro de decisões

Formato curto: contexto → decisão → consequência.

## D1. Biblioteca iCal: `github.com/arran4/golang-ical`

- **Contexto:** o iCal do Moodle tem line folding com TAB, escapes de TEXT e,
  às vezes, linhas fora do padrão RFC 5545.
- **Decisão:** usar a `arran4/golang-ical`, fixada em **v0.3.7**. É a lib mais
  usada em Go e não tem dependências transitivas. A v0.3.8 tinha 2 dias de
  publicada quando a escolha foi feita e ficou de fora por cautela com a cadeia
  de suprimentos.
  - **Alternativa avaliada:** `emersion/go-ical`, que tem uma API boa mas
    depende de `rrule-go`, que não usamos.
- **Consequência:** as datas são interpretadas pelo próprio Prazo, porque a lib
  usa `time.Local` para horários flutuantes, o que deixaria o resultado
  dependente da máquina. Horários sem fuso são lidos como `America/Sao_Paulo`.

## D2. Domínio sem Moodle

- **Contexto:** a primeira versão tinha os templates pt_br e o formato do
  shortname dentro de `domain`.
- **Decisão:** criar o pacote `moodle`, puro e sem I/O, com essas convenções.
  O `domain` fica só com o que vale para qualquer fonte.
- **Consequência:** as duas fontes (iCal e API) reaproveitam a mesma
  interpretação, e o domínio pode ser testado sem nenhum detalhe do Moodle.

## D3. Agrupar Início/Término/conclusão e manter só o Término

- **Contexto:** o Moodle cria até três eventos por atividade, o que polui a
  agenda.
- **Decisão:** manter o Término. O Início fica só com
  `PRAZO_INCLUDE_OPENINGS=true`. A "conclusão esperada" sai quando existe um
  prazo real. Aprovado pelo usuário em 20/10/2026.
- **Consequência:** como o iCal não expõe o id do módulo, a chave do
  agrupamento é disciplina + título normalizado. Se um professor criar duas
  atividades com o mesmo nome na mesma disciplina, elas seriam agrupadas. Na
  Fase 5, a API fornece o id do módulo e a chave fica exata.

## D4. "Prova" criada como tarefa vira prova

- **Contexto:** professores criam provas como tarefas de envio ("Prova 1").
- **Decisão:** quando o template indica tarefa e o nome casa com
  prova/exame/avaliação, o tipo é `exam`.
- **Consequência:** a prova recebe os lembretes de prova (7 dias e 1 dia
  antes), e não os de entrega.

## D5. HTTP "educado" com o servidor da universidade

- **Decisão:** um cliente único (`httpx`) com:
  - timeout por tentativa;
  - retry com backoff exponencial e full jitter, respeitando Retry-After;
  - sem retry em erros 4xx;
  - no máximo **1 requisição simultânea** (semáforo implementado como canal);
  - limite de 20 MB por resposta.
- **Consequência:** não sobrecarrega o Moodle, e uma falha transitória não
  derruba a sincronização.

## D6. Credenciais nunca na URL e nunca no log

- **Decisão:** token e senha do Moodle vão no corpo do POST. O `httpx` limpa a
  URL que o `net/http` embute nas mensagens de erro. O logger redige
  parâmetros e chaves sensíveis. Ver [seguranca.md](seguranca.md).

## D7. Configuração que reporta todos os erros

- **Decisão:** o `config.Load` acumula os erros (`errors.Join`) em vez de parar
  no primeiro. A leitura passa por uma função `getenv` injetável, o que permite
  testar sem mexer no ambiente do processo.
- **Consequência:** quem configura vê tudo o que está errado de uma vez.

## D8. Escopo OAuth do Google: `calendar.app.created` (Fase 2)

- **Contexto:** o Prazo só precisa mexer na agenda que ele mesmo cria.
- **Decisão:** usar `https://www.googleapis.com/auth/calendar.app.created`, e
  não `calendar.events` nem `calendar`.
- **Consequência:** o Prazo não consegue ler nem alterar a agenda principal do
  usuário. Se o token vazar, o estrago fica restrito à agenda "Prazo — UDESC".

## D9. ID de evento determinístico no Google (Fase 2)

- **Decisão:** o id do evento no Google é derivado do `SourceID`, codificado
  em base32hex minúsculo (o Google aceita `a-v` e `0-9`, de 5 a 1024
  caracteres). Um `insert` repetido devolve 409 e vira `update`.
- **Consequência:** a operação é idempotente mesmo que o processo caia no meio
  da sincronização, sem duplicar eventos.

## D10. Imagem distroless e não-root

- **Decisão:**
  - build em etapas, com cache do BuildKit para os módulos Go e para a compilação;
  - imagem final `gcr.io/distroless/static-debian12:nonroot` (~9 MB), rodando como UID 65532;
  - no compose: filesystem `read_only`, `cap_drop: ALL` e `no-new-privileges`.
