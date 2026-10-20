# Segurança

## Segredos

| Segredo | Onde vive | Observação |
|---|---|---|
| URL do iCal (`authtoken`) | `PRAZO_ICAL_URL` | dá acesso ao calendário sem senha; **muda quando a senha do Moodle muda** |
| token do Moodle | `PRAZO_MOODLE_TOKEN` | Fase 5: criptografado em repouso |
| senha do Moodle | **em lugar nenhum** | usada só para obter o token, em memória, e zerada logo depois (`clear`) |
| refresh token do Google | Fase 2: banco, criptografado | AES-256-GCM com a chave `PRAZO_MASTER_KEY` |
| `PRAZO_MASTER_KEY` | só no ambiente | nunca no banco nem no repositório |

Regras:

- O `.env` está no `.gitignore` e no `.dockerignore`.
- A URL do iCal nunca é passada como argumento de linha de comando, porque
  ficaria no histórico do shell. Ela é lida só do ambiente.
- Quando a URL do iCal é inválida, a mensagem de erro **não ecoa** a URL.
- A senha pode entrar pelo terminal (sem eco, `x/term`) ou por
  `-password-stdin`, como no `docker login`.

## Redação de logs (`internal/logging`)

- Parâmetros `authtoken`, `wstoken`, `token`, `password`, `access_token`,
  `refresh_token`, `client_secret` e afins são trocados por `REDACTED`, tanto
  em URLs quanto em texto livre.
- Atributos cujo nome contém `token`, `secret`, `password`, `authorization` ou
  `key` são redigidos inteiros.
- Erros (`error`) passam pela mesma redação antes de serem escritos.
- O `httpx` limpa a URL dentro de `*url.Error`, porque o `net/http` coloca a
  URL completa nas mensagens de erro de rede.

Há testes para cada um desses caminhos (`logging_test.go` e `client_test.go`).

## Comunicação com o Moodle

- Credenciais vão no **corpo** do POST, nunca na query string.
- O probe de `/login/token.php` usa um usuário aleatório inexistente
  (`prazo-doctor-<aleatório>`), sem tocar em nenhuma conta real.
- O cliente se identifica pelo User-Agent `prazo/0.1`.

## Container

- A imagem é distroless, sem shell nem gerenciador de pacotes.
- Roda como UID 65532 (não-root).
- No compose: `read_only: true` (só o volume `/data` é gravável), `tmpfs: /tmp`,
  `cap_drop: [ALL]` e `no-new-privileges`.

## Pendências (fases seguintes)

- Pacote `crypto` com AES-256-GCM e nonce aleatório por registro.
- Rotação de chave: guardar a versão da chave junto do texto cifrado.
- Teste de que nenhum segredo aparece em texto plano no SQLite.
