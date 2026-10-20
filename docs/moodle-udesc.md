# Moodle da UDESC: o que descobrimos

Levantamento feito em **20/10/2026** com `prazo doctor`, numa conta real de
aluno do CCT. Nenhum dado pessoal ou credencial foi registrado aqui.

## Instâncias

A UDESC **não tem um Moodle único.** Cada centro tem o seu:

| Centro | URL | Versão |
|---|---|---|
| CCT (Joinville) | `https://moodle.joinville.udesc.br` | 4.5.12+ |
| geral | `https://www.moodle.udesc.br` (sem `www` redireciona) | não verificada |

Se `PRAZO_MOODLE_URL` não for definida, o Prazo usa o host de `PRAZO_ICAL_URL`.

## Configuração pública (sem login)

A configuração vem de `tool_mobile_get_public_config`, o mesmo endpoint que o
app oficial consulta antes do login.

| Item | CCT | Geral |
|---|---|---|
| idioma do site | pt_br | pt_br |
| web services | ativos | ativos |
| serviço mobile (`moodle_mobile_app`) | ativo | ativo |
| tipo de login do app | 1, usuário e senha | 1, usuário e senha |
| `/login/token.php` | ativo | ativo |
| SSO | não visto | SAML2 "id.udesc/office365" |

O login por senha funcionou na conta de teste do CCT. Contas que entram **só**
pelo SSO provavelmente não conseguem token por senha. Para elas, a Fase 5
prevê o fluxo do app oficial: `admin/tool/mobile/launch.php`, com a pessoa
fazendo login no navegador.

## Web services

Na conta de teste, **446 funções** estavam liberadas, incluindo todas as que o
Prazo usa:

```
core_webservice_get_site_info              core_enrol_get_users_courses
core_calendar_get_action_events_by_timesort core_calendar_get_calendar_upcoming_view
mod_assign_get_assignments                 mod_assign_get_submission_status
mod_quiz_get_quizzes_by_courses            mod_quiz_get_user_attempts
core_completion_get_activities_completion_status
```

`core_calendar_get_action_events_by_timesort` só devolve **eventos ainda
pendentes**. Atividades entregues somem da lista mesmo com o prazo no futuro.
Isso já sinaliza o status de entrega antes mesmo de consultar
`mod_assign_get_submission_status`.

## Exportação iCal

Ela é gerada em Calendário → Exportar → "Todos os eventos". Na amostra real
vieram 54 eventos, de abril a dezembro de 2026.

| Aspecto | Observado | Consequência |
|---|---|---|
| `UID` | `<id>@moodle.joinville.udesc.br`, sem duplicados | é um `SourceID` estável, e o `<id>` é o mesmo id da API |
| `DTSTART`/`DTEND` | sempre UTC (`Z`) e iguais (duração zero) | o prazo é um instante, não um intervalo |
| `CATEGORIES` | shortname da disciplina; ausente em eventos pessoais | daí sai a sigla |
| `DESCRIPTION` | HTML convertido em texto, com rodapé `Links: ------ [1] url` | o rodapé é limpo; a URL da atividade aparece só às vezes |
| `URL`, `LOCATION` | ausentes | o link é montado de outro jeito (ver abaixo) |
| line folding | continuação com **TAB** e linhas de continuação vazias | o parser trata; há teste com o arquivo real |
| horários | existem prazos reais às 02:05 (horário local) | vêm do Moodle e são mantidos |

**O iCal não informa o tipo do módulo** (assign, quiz...). O tipo é deduzido
do texto que o Moodle adiciona ao nome do evento, e na Fase 5 vai ser
confirmado pela API (`modulename`).

**O link da atividade** só aparece na descrição quando o filtro "auto-link de
nomes de atividade" do Moodle atua (15 de 54 eventos). Links para `resource`,
`url`, `page`, `folder`, `label` e `book` são ignorados, porque costumam ser
slides citados no enunciado. Sem link, o Prazo usa o dia no calendário do
Moodle (`/calendar/view.php?view=day&time=...`).

## Shortname das disciplinas (CCT)

```
2026/2_CCT_CCI251-03A_POO0002
└─┬──┘ └┬┘ └───┬────┘ └──┬──┘
semestre centro  turma   código → sigla POO
```

A regra fica em `moodle.CourseCode`: pega o primeiro bloco de letras do último
segmento. Para outros formatos há fallback.

## Templates de nome de evento

O Moodle grava o nome do evento **já traduzido** no idioma do site no
momento da criação. As regras ficam em `internal/moodle/eventname.go`.

| Template (pt_br) | Origem | Tipo / fase |
|---|---|---|
| `X está marcado(a) para esta data` | mod_assign | entrega / due |
| `X está com avaliação marcada para esta data` | mod_assign | correção (descartado) |
| `X é devido` | mod_forum | fórum / due |
| `Início de X` / `Término de X` | quiz, feedback, choice, data, scorm | por palavra-chave / open, close |
| `X deve estar concluído` | completion | por palavra-chave / expect |
| `X (Data limite)` | mod_groupselect (plugin do CCT) | atividade / due |
| `X submission is due` | inglês, visto em AOC | entrega / due |

Quando o template é ambíguo (`Início de`/`Término de`), o tipo sai de
palavras-chave no nome:

- "prova", "exame" ou "avaliação" → prova;
- "questionário", "quiz" ou "teste" → questionário;
- "tarefa", "lista", "trabalho" ou "upload" → entrega.

Uma **tarefa** chamada "Prova 1" também vira prova, para receber os lembretes
de prova.

Na amostra real, **todos os 53 eventos de disciplina** casaram com alguma
regra. O único sem regra é um evento pessoal, que é o esperado.

## Eventos múltiplos por atividade

As tarefas de Cálculo (CDI) geram três eventos cada: "Início de TAREFA n",
"Término de TAREFA n" e "TAREFA n deve estar concluído". Depois do
agrupamento, sobra **só o Término**. Os 54 eventos da amostra viram 40 prazos.

## Fixtures

| Arquivo | Origem |
|---|---|
| `testdata/moodle_sintetico.ics` | montado à mão no formato do Moodle 4.x |
| `testdata/moodle_cct_anon.ics` | 12 eventos reais do CCT, com as descrições anonimizadas e o formato preservado (TAB folding, escapes) |
