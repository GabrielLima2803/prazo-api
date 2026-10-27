# Privacidade (LGPD)

O Prazo trata dados pessoais de quem entra pelo app, então segue a LGPD
(Lei 13.709/2018), a [Política de Dados do Usuário dos Serviços de API do
Google](https://developers.google.com/terms/api-services-user-data-policy) e
as regras das lojas de aplicativos. A política pública fica em
`GET /privacidade` (texto em `internal/api/privacy.go`).

## O que fica guardado

Sem banco de dados: cada item é um arquivo cifrado (AES-256-GCM) em
`PRAZO_DATA_DIR`.

| Dado | Chave | Por quê |
|---|---|---|
| id, host, userid do Moodle e nome | `users` | separar as contas; cumprimentar no app |
| hash das sessões do app | `app_sessions` | autenticar o aparelho (validade de 180 dias) |
| token do Moodle e URL do calendário | `u<id>_moodle_account` | ler prazos e status de entrega |
| token do Google | `u<id>_google_token` | escrever na agenda do Prazo |
| id da agenda | `u<id>_google_calendar_id` | reaproveitar a mesma agenda |
| preferências | `u<id>_preferences` | cores, lembretes, ocultos |
| id do evento, hash e data de cada prazo | `u<id>_sync_state` | diff e remoção segura |
| prazos vistos como pendentes | `u<id>_pending_seen` | marcar entregas (D22) |

Nunca guardados: a senha do Moodle e o conteúdo dos prazos (ficam só em
memória). Os logs não têm senhas, tokens nem o conteúdo dos prazos.

## Direitos do titular (art. 18)

| Direito | Como |
|---|---|
| acesso e portabilidade | Ajustes → Baixar meus dados (`GET /api/v1/account/export`) |
| eliminação | Ajustes → Excluir conta e dados (`DELETE /api/v1/account`) |
| revogação do consentimento (Google) | Ajustes → Desconectar (revoga no Google) |
| correção | entrar de novo atualiza nome e credenciais |
| informação | página `/privacidade` |

A exclusão apaga todas as chaves de `store.UserKeys`, tira o usuário da
lista, revoga o token no Google e encerra todas as sessões. A agenda criada
no Google continua na conta da pessoa, que decide se a apaga.

## Checklist antes de abrir para mais gente

Itens que dependem de você, fora do código:

- [ ] Preencher `PRAZO_PRIVACY_OPERATOR` e `PRAZO_PRIVACY_CONTACT` (um e-mail
      que você leia; a LGPD exige um canal para os pedidos).
- [ ] No Google Cloud, tela de consentimento OAuth: informar a URL
      `<PRAZO_PUBLIC_URL>/privacidade`, publicar o app ("Em produção") e
      pedir a verificação. Em modo Teste, o refresh token expira em 7 dias e
      só os testadores cadastrados entram.
- [ ] Nas lojas: usar a mesma URL como política de privacidade; preencher a
      "Segurança dos dados" (Google Play) e os "Rótulos de privacidade" (App
      Store) com os dados da tabela acima; informar que a exclusão da conta
      é feita no próprio app.
- [ ] Deixar `PRAZO_CORS_ORIGINS` vazio em produção.
- [ ] Guardar `PRAZO_MASTER_KEY` fora do servidor (gerenciador de senhas):
      sem ela os dados ficam ilegíveis, e com ela qualquer cópia do disco é
      legível.
- [ ] Se possível, pedir a um profissional de direito uma revisão do texto de
      `/privacidade`. Ele foi escrito com base na LGPD, mas não é parecer
      jurídico.
- [ ] Em caso de incidente com risco aos usuários: avisar os usuários e a
      ANPD (art. 48).
