# Prazo

Sincroniza prazos, entregas, provas e eventos do Moodle da UDESC com uma agenda
dedicada no Google Agenda: título padronizado, cor por disciplina, lembretes por
tipo, filtros e status de entrega.

> Em construção. Fases 0 a 2 concluídas (leitura do Moodle e sincronização
> com o Google Agenda), com API e app móvel em andamento. Detalhes em [docs/roadmap.md](docs/roadmap.md).

```
$ prazo sync --dry-run
QUANDO           FALTA   TÍTULO
qua 21/10 23:59  1d 4h   [EDA] Entrega: UPLOAD da Tarefa-1 FILA DE PRIORIDADE
qui 22/10 23:59  2d 4h   [POO] Entrega: Aula Prática 8 - Persistência de Dados em Arquivo
ter 27/10 02:04  6d 6h   [AMS] Atividade: Definição de grupo para MVP
```

## Início rápido

1. No Moodle: **Calendário → Importar ou exportar calendários → Exportar calendário**,
   "Todos os eventos", "Eventos recentes e próximos", **Obter URL do calendário**.
   A URL contém um `authtoken`: trate como senha.
2. Configure e rode:

```sh
cp .env.example .env               # cole a URL em PRAZO_ICAL_URL (entre aspas simples)
go run ./cmd/prazo keygen          # cole o resultado em PRAZO_MASTER_KEY
set -a; . ./.env; set +a
go run ./cmd/prazo doctor          # diagnóstico
go run ./cmd/prazo sync --dry-run  # próximos 30 dias, sem escrever nada
```

3. Google Agenda: crie uma credencial OAuth **"App para computador"** no Google
   Cloud Console (passo a passo em [docs/desenvolvimento.md](docs/desenvolvimento.md#google-agenda)),
   salve o JSON em `secrets/google-oauth.json` e rode:

```sh
go run ./cmd/prazo auth   # abre o consentimento do Google; o token fica cifrado em data/
go run ./cmd/prazo sync   # cria a agenda "Prazo — UDESC" e envia os prazos
```

## Documentação

| | |
|---|---|
| [Arquitetura](docs/arquitetura.md) | pacotes, dependências, fluxo e tipos centrais |
| [Moodle da UDESC](docs/moodle-udesc.md) | instâncias, formato do iCal, templates de título |
| [Decisões](docs/decisoes.md) | o porquê das escolhas técnicas |
| [Segurança](docs/seguranca.md) | segredos, redação de logs, container |
| [Desenvolvimento](docs/desenvolvimento.md) | variáveis de ambiente, testes, Docker, convenções |
| [Roadmap](docs/roadmap.md) | o que foi feito e os próximos passos |
