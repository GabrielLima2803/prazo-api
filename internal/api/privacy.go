package api

import (
	"html/template"
	"net/http"
)

// privacyVersion muda a cada alteração do texto; a página mostra a data.
const privacyVersion = "27 de outubro de 2026"

// privacyPage é a política de privacidade e os termos de uso, públicos para
// o app, a tela de consentimento do Google e as lojas de aplicativos.
func (s *server) privacyPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP)
	privacyTmpl.Execute(w, privacyData{Privacy: s.Privacy, Version: privacyVersion, CalendarName: s.CalendarName})
}

type privacyData struct {
	Privacy
	Version      string
	CalendarName string
}

var privacyTmpl = template.Must(template.New("privacy").Parse(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Prazo — Privacidade e termos de uso</title>
<style>
:root{color-scheme:light dark;--bg:#F4F5F9;--fg:#0E1220;--muted:#5F6880;--card:#fff;--line:#E3E6EE;--accent:#5B3DF5}
@media (prefers-color-scheme:dark){:root{--bg:#0B0D12;--fg:#F2F4F8;--muted:#8B93A7;--card:#141821;--line:#252B3B;--accent:#8B6CFF}}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.6 system-ui,sans-serif}
main{max-width:720px;margin:0 auto;padding:32px 16px 64px}
h1{font-size:28px;margin:0 0 4px}h2{font-size:19px;margin:32px 0 8px}
p,li{color:var(--fg)}small,.muted{color:var(--muted)}
section{background:var(--card);border:1px solid var(--line);border-radius:16px;padding:4px 20px 16px;margin-top:16px}
a{color:var(--accent)}code{font-size:14px}
</style></head><body><main>
<h1>Privacidade e termos de uso</h1>
<p class="muted">Prazo · versão de {{.Version}}</p>

<section>
<h2>Quem cuida dos seus dados</h2>
<p>O Prazo é um projeto independente, <strong>não oficial da UDESC</strong>, que mostra os prazos do seu Moodle e os leva para uma agenda do Google.
O controlador dos dados, nos termos da LGPD (Lei 13.709/2018), é <strong>{{if .Operator}}{{.Operator}}{{else}}quem mantém este servidor{{end}}</strong>.
{{if .Contact}}Para qualquer pedido sobre os seus dados, escreva para <a href="mailto:{{.Contact}}">{{.Contact}}</a>.{{end}}</p>
</section>

<section>
<h2>Quais dados tratamos e por quê</h2>
<ul>
<li><strong>Senha do Moodle:</strong> usada uma única vez, no login, para obter um token de acesso. <strong>Nunca é guardada</strong> nem registrada em log.</li>
<li><strong>Token do Moodle e URL de exportação do calendário:</strong> permitem ler os seus prazos e o status de entrega. Ficam cifrados (AES-256-GCM).</li>
<li><strong>Identificação no Moodle:</strong> número de usuário, nome completo e endereço do Moodle, para separar a sua conta das demais e cumprimentar você no app.</li>
<li><strong>Prazos e atividades:</strong> lidos do Moodle a cada consulta e mantidos só em memória. Para sincronizar a agenda, guardamos apenas o identificador de cada evento, um resumo criptográfico (hash) e a data.</li>
<li><strong>Google Agenda:</strong> um token com acesso <em>somente</em> à agenda “{{.CalendarName}}”, criada pelo Prazo, e o identificador dessa agenda. Não lemos nem alteramos nenhuma outra agenda, contato ou e-mail. O token fica cifrado.</li>
<li><strong>Preferências:</strong> cores, lembretes e o que fica oculto.</li>
<li><strong>Sessões do app:</strong> guardamos só um hash do token de cada aparelho. As sessões expiram em 180 dias.</li>
<li><strong>Registros técnicos:</strong> data, rota e resultado das requisições, sem senhas, tokens nem o conteúdo dos prazos.</li>
</ul>
<p>Não usamos banco de dados: cada informação fica num arquivo cifrado no servidor. Não há publicidade, venda, perfilamento nem uso dos dados para treinar inteligência artificial.</p>
</section>

<section>
<h2>Base legal</h2>
<p>Tratamos os dados para executar o serviço que você pediu ao entrar no app (LGPD, art. 7º, V). A conexão com o Google depende do seu consentimento na tela do Google (art. 7º, I), que pode ser retirado a qualquer momento.</p>
</section>

<section>
<h2>Com quem os dados são compartilhados</h2>
<ul>
<li><strong>Moodle da UDESC:</strong> para ler os seus prazos, com as credenciais que você forneceu.</li>
<li><strong>Google:</strong> para criar e atualizar os eventos na agenda do Prazo. O uso e a transferência, para qualquer outro app, das informações recebidas das APIs do Google seguem a <a href="https://developers.google.com/terms/api-services-user-data-policy">Política de Dados do Usuário dos Serviços de API do Google</a>, incluindo os requisitos de Uso Limitado.</li>
</ul>
<p>Mais ninguém recebe os seus dados.</p>
</section>

<section>
<h2>Por quanto tempo</h2>
<p>Enquanto você usar o Prazo. Ao excluir a conta, apagamos na hora tudo o que está no servidor e encerramos todas as sessões. Não há cópias de segurança dos seus dados.</p>
</section>

<section>
<h2>Seus direitos</h2>
<p>A LGPD (art. 18) garante a você confirmação, acesso, correção, portabilidade, eliminação, informação sobre compartilhamento e revogação do consentimento. No app:</p>
<ul>
<li><strong>Ajustes → Baixar meus dados:</strong> cópia, em JSON, de tudo o que guardamos sobre você.</li>
<li><strong>Ajustes → Excluir conta e dados:</strong> apaga tudo e revoga o acesso ao Google.</li>
<li><strong>Ajustes → Google Agenda → Desconectar:</strong> revoga só o acesso ao Google.</li>
<li><strong>Sair:</strong> encerra a sessão deste aparelho; a agenda continua sendo atualizada até você excluir a conta.</li>
</ul>
<p>Você também pode revogar os acessos diretamente: no Google, em <a href="https://myaccount.google.com/permissions">myaccount.google.com/permissions</a>; no Moodle, em Preferências → Chaves de segurança, redefinindo a chave do serviço móvel.</p>
</section>

<section>
<h2>Segurança</h2>
<p>Comunicação com o app e com o Moodle por HTTPS, credenciais cifradas em repouso, permissão mínima no Google, limite de tentativas de login e registros sem segredos. Se acontecer um incidente que possa trazer risco a você, avisaremos você e a ANPD.</p>
</section>

<section>
<h2>Termos de uso</h2>
<ul>
<li>O Prazo é gratuito e serve de apoio à organização. <strong>O Moodle continua sendo a fonte oficial dos prazos:</strong> confira sempre por lá, porque a sincronização pode atrasar ou falhar.</li>
<li>Use o Prazo só com a sua própria conta do Moodle.</li>
<li>O serviço pode ficar fora do ar ou ser encerrado. Se for encerrado, avisaremos com antecedência e apagaremos os dados.</li>
<li>A data no topo mostra a última alteração desta página; mudanças relevantes serão comunicadas aos usuários.</li>
</ul>
</section>
</main></body></html>`))
