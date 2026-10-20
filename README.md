# Prazo

Sincroniza prazos, entregas, provas e eventos do Moodle da UDESC com uma agenda
dedicada no Google Agenda, com título padronizado, cor por disciplina,
lembretes por tipo e status de entrega.

> Em construção. Fase atual: **0 — Descoberta** (`prazo doctor`).

## Requisitos

- Go 1.25+

## Obter a URL do iCal no Moodle

1. Entre no Moodle → **Calendário** → **Importar ou exportar calendários** → **Exportar calendário**.
2. Em "Eventos a exportar" escolha **Todos os eventos**; em "Período", **Eventos recentes e próximos**.
3. Clique em **Obter URL do calendário** e copie a URL.

A URL contém `authtoken`, que dá acesso ao seu calendário sem senha.
**Trate como senha**: guarde no `.env`, nunca passe como argumento de linha de
comando (vai para o histórico do shell) e nunca cole em issue/chat.

## Diagnóstico (`prazo doctor`)

```sh
cp .env.example .env   # preencha PRAZO_ICAL_URL
set -a; . ./.env; set +a
go run ./cmd/prazo doctor
```

O relatório mostra: quantidade de eventos, formato dos UIDs, propriedades iCal
presentes, disciplinas (`CATEGORIES`), quais regras de título casaram, títulos
não reconhecidos e uma tabela dos próximos 30 dias. Em seguida consulta a
configuração pública do Moodle e testa se `/login/token.php` está ativo (com um
usuário aleatório inexistente, sem credencial real).

Flags úteis:

| flag | o que faz |
|---|---|
| `-ics arquivo.ics` | lê um arquivo local em vez da URL |
| `-days 60` | janela da tabela de próximos |
| `-user SEU_USUARIO` | pede a senha, obtém um token temporário e lista as funções de web service liberadas. Nada é salvo. |
| `-skip-ws` | não consulta web services |
| `-probe-login=false` | não faz o teste de `/login/token.php` |

### Token via SSO (contas id.udesc / Office 365)

Se sua conta entra pelo botão "Login via id.udesc/office365", a senha não
funciona em `/login/token.php`. Isso será tratado na Fase 5; até lá, o iCal
basta.
