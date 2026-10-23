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

## D8. Escopo OAuth do Google: `calendar.app.created`

- **Contexto:** o Prazo só precisa mexer na agenda que ele mesmo cria.
- **Decisão:** usar `https://www.googleapis.com/auth/calendar.app.created`, e
  não `calendar.events` nem `calendar`.
- **Consequência:** o Prazo não consegue ler nem alterar a agenda principal do
  usuário. Se o token vazar, o estrago fica restrito à agenda "Prazo — UDESC".

## D9. ID de evento determinístico no Google

- **Decisão:** o id do evento no Google é `prazo:<SourceID>` codificado em
  base32hex minúsculo (o Google aceita `a-v` e `0-9`, de 5 a 1024
  caracteres; acima disso, usa o SHA-256). Um `insert` repetido devolve 409 e
  vira `update`, o que também restaura um evento apagado à mão.
- **Consequência:** a operação é idempotente mesmo que o processo caia no meio
  da sincronização, sem duplicar eventos.

## D10. Imagem distroless e não-root

- **Decisão:**
  - build em etapas, com cache do BuildKit para os módulos Go e para a compilação;
  - imagem final `gcr.io/distroless/static-debian12:nonroot` (~9 MB), rodando como UID 65532;
  - no compose: filesystem `read_only`, `cap_drop: ALL` e `no-new-privileges`.

## D11. Google API oficial e Go 1.26

- **Contexto:** a especificação pede `google.golang.org/api/calendar/v3` e
  `x/oauth2`. A versão fixada (v0.301.0, publicada há mais de duas semanas)
  exige `x/oauth2` v0.37, que exige Go 1.26.
- **Decisão:** subir o projeto para Go 1.26 e usar a biblioteca oficial.
- **Consequência:** a biblioteca traz dependências indiretas pesadas (gRPC,
  OpenTelemetry), e a imagem foi de ~9 MB para ~19 MB. Em troca, os tipos da
  API, os erros (`googleapi.Error` com `reason`) e o OAuth seguem os da
  própria Google. Só o pacote `calendar/google` importa essa biblioteca.

## D12. Horário do evento: real quando existe, 30 min terminando no prazo

- **Contexto:** no Moodle, quase todo prazo é um instante (DTSTART = DTEND).
  Um evento de duração zero quase não aparece na grade da agenda.
- **Decisão:**
  - prazo pontual → bloco de 30 min que **termina** no prazo, para que a hora
    final mostrada seja o limite real;
  - Início e Término da mesma atividade a até 12 h → um evento com o intervalo
    real (prova online das 19h às 21h);
  - eventos com duração própria mantêm a duração.
  Aprovado pelo usuário em 23/10/2026.
- **Consequência:** questionários abertos por vários dias continuam sendo só
  o Término. Só provas com intervalo real marcam a agenda como "ocupado".

## D13. Horizonte: tudo o que o Moodle exporta

- **Contexto:** o usuário quer o semestre inteiro, e não só os próximos dias.
  A exportação "Eventos recentes e próximos" do CCT já cobre o semestre.
- **Decisão:** sincronizar tudo, com `PRAZO_SYNC_SINCE` (AAAA-MM-DD) opcional
  para cortar prazos antigos.
- **Consequência:** prazos que saem da exportação por serem antigos **não**
  podem ser apagados da agenda. A Fase 3 só apaga o que sumiu e ainda estava
  no futuro.

## D14. Segredos em arquivos cifrados até o SQLite

- **Decisão:** a interface `store.KV` com a implementação `filekv`: um arquivo
  `<chave>.enc` por segredo, permissão 0600, escrita atômica (temporário +
  rename), AES-256-GCM com o nome da chave como AAD.
- **Consequência:** a Fase 3 troca a implementação por SQLite sem mexer no
  `sync` nem no `calendar/google`. O AAD impede trocar o arquivo de um segredo
  pelo de outro.

## D15. OAuth de app instalado: loopback + PKCE

- **Decisão:** um servidor temporário em `127.0.0.1`, com `state` aleatório
  (comparado em tempo constante), PKCE S256, `access_type=offline` e
  `prompt=consent` para garantir o refresh token. O comando confere se o
  escopo foi realmente concedido (o Google permite desmarcar a caixa).
- **Consequência:** funciona no terminal sem colar código. No Docker, o
  servidor escuta em `0.0.0.0:8085` com a porta publicada só em `127.0.0.1`,
  e o redirect continua sendo `127.0.0.1`.
- **App móvel (futuro):** o mesmo projeto do Google Cloud vai ganhar
  credenciais "Aplicativo da Web" (para o servidor) e Android/iOS (para o
  login no app). A credencial de computador continua servindo à linha de
  comando.
