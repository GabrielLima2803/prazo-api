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
| 2 | Google Agenda (`prazo auth`, `prazo sync`) | ✅ concluída |
| 3 | Estado, diff e remoção segura | ✅ concluída sem banco (estado em arquivos cifrados); SQLite adiado |
| 4 | Serviço (`prazo serve`), Docker de produção | ✅ concluída |
| 5 | Web services e status de entrega (✅) | ✅ login pelo Moodle e status de entrega |
| 6 | ~~Resumos via Telegram~~ | removida a pedido |
| 7 | App móvel e multiusuário | ✅ app ([prazo-app](https://github.com/GabrielLima2803/prazo-app)) e multiusuário; falta publicar os builds |

## Índice

| Documento | Conteúdo |
|---|---|
| [arquitetura.md](arquitetura.md) | camadas, pacotes, regras de dependência, fluxo de dados, tipos centrais |
| [moodle-udesc.md](moodle-udesc.md) | o que descobrimos sobre as instâncias Moodle da UDESC (dados reais) |
| [decisoes.md](decisoes.md) | registro de decisões técnicas e o porquê de cada uma |
| [seguranca.md](seguranca.md) | tratamento de segredos, redação de logs, container |
| [privacidade.md](privacidade.md) | LGPD: dados tratados, direitos do titular e checklist antes de publicar |
| [desenvolvimento.md](desenvolvimento.md) | como rodar, testar, construir a imagem e convenções do código |
| [api.md](api.md) | rotas da API usada pelo app, preferências e cache |
| [roadmap.md](roadmap.md) | o que foi entregue em cada fase e os próximos passos detalhados |
