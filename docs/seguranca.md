# Segurança

## Segredos

| Segredo | Onde vive | Observação |
|---|---|---|
| URL do iCal (`authtoken`) | `PRAZO_ICAL_URL` | dá acesso ao calendário sem senha; **muda quando a senha do Moodle muda** |
| token do Moodle | `PRAZO_MOODLE_TOKEN` | Fase 5: criptografado em repouso |
| senha do Moodle | **em lugar nenhum** | usada só para obter o token, em memória, e zerada logo depois (`clear`) |
| refresh token do Google | `PRAZO_DATA_DIR/google_token.enc` | AES-256-GCM com a chave `PRAZO_MASTER_KEY`; revogado no Google ao desconectar ou excluir a conta |
| credencial OAuth (`client_secret`) | `secrets/google-oauth.json` (0600) | ignorada pelo git e pelo build do Docker |
| `PRAZO_API_TOKEN` | só no ambiente (`.env`) e no Keychain/Keystore do celular | comparado em tempo constante; nunca vai em cookie nem em URL |
| `PRAZO_MASTER_KEY` | só no ambiente (`.env`, 0600) | nunca no disco de dados nem no repositório |

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

## Criptografia (`internal/storage`)

- AES-256-GCM, nonce aleatório de 12 bytes por gravação.
- Formato: `versão (1 byte) | nonce | ciphertext+tag`. A versão permite trocar
  o algoritmo ou a chave no futuro.
- O nome do segredo entra como AAD: o arquivo de um segredo não pode ser
  renomeado para se passar por outro.
- Chave errada, byte alterado, AAD diferente ou arquivo truncado dão o mesmo
  erro (`ErrCiphertext`), sem detalhes que ajudem um atacante.
- Testes: ida e volta, nonce aleatório, adulteração, chave errada, e "o
  segredo não aparece em texto plano no disco".

## Google

- Escopo único `calendar.app.created`: o Prazo não lê nem altera nenhuma
  agenda além da que ele mesmo criou (D8).
- OAuth com PKCE e `state` aleatório; a resposta é recebida só em `127.0.0.1`.
- A URL de autorização exibida no terminal não contém segredos (só o
  `client_id`, que é público).
- Token revogado ou expirado vira `calendar.ErrAuth`, e o sync para na hora,
  em vez de tentar evento por evento.
- Enquanto o app do Google Cloud estiver em modo **Teste**, o refresh token
  expira em 7 dias. Para uso contínuo, publique o app (modo "Em produção").

## API do app

- Sessões do app: token aleatório de 52 caracteres entregue só no login;
  no disco fica apenas o SHA-256 (`app_sessions.enc`, cifrado). Máximo de 10
  aparelhos, validade de 180 dias; sair revoga a sessão do aparelho.
- Login só nos Moodles de `PRAZO_MOODLE_HOSTS`. A lista vazia recusa todos
  (falha fechada), para o servidor nunca buscar um endereço qualquer (SSRF).
  O cliente HTTP só segue redirecionamentos para HTTPS.
- Limites de login: 20 por minuto no total e 5 a cada 15 minutos por conta
  (Moodle + usuário), para ninguém testar senhas de outra pessoa por aqui.

- Toda rota fora `/healthz` exige o token Bearer; sem ele, 401 sem detalhes.
- Por padrão escuta só em `127.0.0.1`. Para o celular, `0.0.0.0:8080` na
  rede local: o tráfego é HTTP puro, então qualquer um na mesma rede Wi-Fi
  poderia ver o token. Em casa é aceitável; fora dela, use HTTPS (Tailscale
  ou um proxy com TLS).
- CORS desligado por padrão: o app nativo não precisa. Para o Expo web, libere
  as origens em `PRAZO_CORS_ORIGINS` só em desenvolvimento.
- Cabeçalhos em todas as respostas: `nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy: no-referrer`, HSTS, CSP restritiva e `Cache-Control:
  no-store` (as respostas trazem dados pessoais).
- Erros 500 respondem uma mensagem genérica (`code: internal`); o detalhe
  vai só para o log. A página de callback do Google também não repete a
  resposta do Google.
- Corpo do `PUT` limitado a 64 KB, campos desconhecidos rejeitados.
- Com o Tailscale Funnel, a API fica na internet por HTTPS. Continua exigindo
  o token (32+ bytes aleatórios); só `/healthz` e o callback do OAuth são
  públicos.
- Callback do Google: `state` aleatório de uso único com validade de 10 min,
  PKCE S256 e escopo conferido. O retorno ao app só aceita esquemas de app
  (`exp://`, `prazo://`), nunca `http(s)`, para não virar redirecionador aberto.
- O token guarda qual credencial o emitiu (`installed` ou `web`), porque o
  refresh só funciona com a mesma credencial.

## Privacidade (LGPD)

Detalhes em [privacidade.md](privacidade.md). Em resumo: sem banco de dados,
dados mínimos e cifrados, página pública `/privacidade`, exportação
(`GET /api/v1/account/export`) e exclusão da conta (`DELETE /api/v1/account`)
pelo próprio app.

## Comunicação com o Moodle

- Credenciais vão no **corpo** do POST, nunca na query string.
- O probe de `/login/token.php` usa um usuário aleatório inexistente
  (`prazo-doctor-<aleatório>`), sem tocar em nenhuma conta real.
- O cliente se identifica pelo User-Agent `prazo/0.1`.

## Container

- A imagem é distroless, sem shell nem gerenciador de pacotes.
- Roda como UID 65532 (não-root).
- A credencial OAuth entra como `secret` do compose, montada só leitura em
  `/run/secrets/google_oauth`. No host, o arquivo continua 0600 e só o UID
  65532 ganha leitura via ACL (`setfacl -m u:65532:r`), sem abrir para
  "outros".
- No compose: `read_only: true` (só o volume `/data` é gravável), `tmpfs: /tmp`,
  `cap_drop: [ALL]` e `no-new-privileges`.

## Pendências (fases seguintes)

- Rotação de chave: hoje a versão identifica o formato; falta um id de chave
  para reabrir segredos antigos com a chave anterior.
- Excluir automaticamente contas sem uso há muito tempo (hoje ficam até o
  usuário excluir).
