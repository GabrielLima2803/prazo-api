# Documentação do Prazo

O Prazo sincroniza prazos, entregas, provas e eventos do Moodle da UDESC com
uma agenda dedicada no Google Agenda. Ele vai além de "assinar o iCal" no
Google:

- sincroniza rápido;
- padroniza os títulos;
- dá uma cor para cada disciplina;
- configura lembretes por tipo de atividade;
- filtra o que não interessa;
- marca as entregas já feitas.

## Estado atual

| Fase | Escopo | Status |
|---|---|---|
| 0 | Descoberta (`prazo doctor`) | ✅ concluída |
| 1 | Leitura e normalização (`prazo sync --dry-run`) | ✅ concluída |
| 2 | Google Agenda (`prazo auth`, `prazo sync`) | ⏭ próxima |
| 3 | Estado (SQLite), diff e deletes seguros | pendente |
| 4 | Serviço (`prazo serve`), healthcheck e Docker de produção | pendente |
| 5 | Web services e status de entrega (✅) | pendente |
| 6 | Resumos via Telegram | pendente |
| 7 | Multiusuário (opcional) | pendente |

## Índice

| Documento | Conteúdo |
|---|---|
| [arquitetura.md](arquitetura.md) | camadas, pacotes, regras de dependência, fluxo de dados, tipos centrais |
| [moodle-udesc.md](moodle-udesc.md) | o que descobrimos sobre as instâncias Moodle da UDESC (dados reais) |
| [decisoes.md](decisoes.md) | registro de decisões técnicas e o porquê de cada uma |
| [seguranca.md](seguranca.md) | tratamento de segredos, redação de logs, container |
| [desenvolvimento.md](desenvolvimento.md) | como rodar, testar, construir a imagem e convenções do código |
| [roadmap.md](roadmap.md) | o que foi entregue em cada fase e os próximos passos detalhados |
