# API do app (`prazo serve`)

JSON sobre HTTP, consumida pelo [prazo-app](https://github.com/GabrielLima2803/prazo-app).

## Autenticação

O app entra com o Moodle: `POST /auth/login` (`{baseUrl, username, password}`)
ou `POST /auth/ical` (`{icalUrl}`) devolvem `{token, moodle}`. O token é uma
sessão do app (até 10 aparelhos, validade de 180 dias; só o hash SHA-256 fica
no disco) e vai em `Authorization: Bearer <token>` nas rotas de `/api/v1`.
`POST /api/v1/auth/logout` encerra a sessão do aparelho.

- Cada conta do Moodle tem o próprio espaço. Só os Moodles de
  `PRAZO_MOODLE_HOSTS` são aceitos (403 `host_not_allowed`), até
  `PRAZO_MAX_USERS` usuários (403 `too_many_users`).
- Limites de login (429 `too_many_attempts`): 20 tentativas por minuto no
  servidor todo e 5 a cada 15 minutos por conta.
- O `PRAZO_API_TOKEN` continua valendo em `/api/v1`, para o terminal e
  administração.

```sh
set -a; . ./.env; set +a
PRAZO_HTTP_ADDR=0.0.0.0:8080 go run ./cmd/prazo serve
curl -H "Authorization: Bearer $PRAZO_API_TOKEN" localhost:8080/api/v1/deadlines
```

O servidor sincroniza ao subir e depois a cada `PRAZO_SYNC_INTERVAL` (±10%).
Sem autorização do Google, a API funciona do mesmo jeito: o app mostra os
prazos e avisa que falta o `prazo auth`.

| Rota | Descrição |
|---|---|
| `GET /healthz` | público; `{"ok": true}` |
| `GET /api/v1/status` | Google autorizado?, última execução (contagens e erro), próxima execução |
| `GET /api/v1/meta` | paleta do Google (colorId → hex) e tipos com os lembretes padrão |
| `GET /api/v1/deadlines` | todos os prazos normalizados, com cor, lembretes e `hidden` já resolvidos pelas preferências. `?refresh=1` relê o Moodle |
| `GET /api/v1/courses` | disciplinas com cor, total, pendentes, próximo prazo e se está oculta |
| `GET /api/v1/preferences` | preferências atuais (coleções vazias, nunca `null`) |
| `PUT /api/v1/preferences` | substitui as preferências; 422 se inválidas, 400 para JSON desconhecido |
| `POST /api/v1/sync` | sincroniza agora e devolve o resultado; 409 se já houver uma em andamento |
| `GET /api/v1/moodle` | conta conectada (nome, site, método); nunca devolve a URL do iCal nem o token |
| `POST /api/v1/google/auth` | `{returnTo?}` → `{authUrl}`; 501 `google_app_auth_unavailable` sem `PRAZO_PUBLIC_URL` e credencial web |
| `DELETE /api/v1/google` | revoga o acesso no Google e apaga o token; `revoked: false` se o Google não confirmou |
| `GET /api/v1/account/export` | cópia dos dados do usuário em JSON (LGPD, portabilidade), sem credenciais |
| `DELETE /api/v1/account` | exclui a conta: revoga o Google, apaga todos os dados do usuário e encerra todas as sessões |
| `POST /api/v1/auth/logout` | encerra a sessão deste aparelho |
| `GET /oauth/google/callback` | **público**: o Google devolve o código aqui; o `state` (uso único, 10 min) prova que a autorização começou no app autenticado |
| `GET /privacidade` | **público**: política de privacidade e termos de uso (LGPD, Google, lojas) |

Erros trazem um `code` estável além da mensagem: `moodle_not_connected` (409 em
`/deadlines` e `/courses`), `invalid_login`, `export_unavailable`,
`service_unavailable`, `invalid_url`, `not_icalendar`, `empty_feed`,
`moodle_unreachable`, `moodle_reauth`, `too_many_attempts`, `internal`.
Erros internos (500, `internal`) trazem só uma mensagem genérica; o detalhe
fica no log do servidor.

## Senha do Moodle trocada

O authtoken da URL do iCal é derivado da senha. Quando ela muda, o Moodle
responde `200` com o texto exato `Invalid authentication` (diferente da
página HTML de manutenção). O servidor então:

1. marca a conta (`needsReauth`) e para de chamar o Moodle até o próximo login;
2. não serve o plano antigo do cache, para não esconder que a sincronização parou;
3. responde `409 moodle_reauth` em `/deadlines` e `/courses`.

O app volta para o login com o aviso "Sua senha do Moodle mudou"; entrar de
novo gera a URL nova e mantém as preferências e a agenda.

## Preferências

```json
{
  "hiddenCourses": ["CDI"],
  "hiddenTypes": ["forum"],
  "courseColors": { "POO": "11" },
  "reminders": { "assign": [1440, 120], "exam": [] }
}
```

- `courseColors`: colorId de 1 a 11 (paleta do Google Agenda).
- `reminders`: minutos antes do prazo; até 5 por tipo, até 4 semanas. Lista
  vazia desliga os lembretes daquele tipo; tipo ausente usa o padrão.
- Ficam cifradas no `PRAZO_DATA_DIR` (`preferences.enc`) e valem tanto para o
  app quanto para o sync com o Google.

## Cache

`/deadlines` e `/courses` usam o último plano lido por até 5 minutos, para não
bater no Moodle a cada abertura do app. Se o Moodle estiver fora, a API devolve
o último plano conhecido.
