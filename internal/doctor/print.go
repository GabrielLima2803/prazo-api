package doctor

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"prazo/internal/collections"
	"prazo/internal/logging"
)

type printer struct{ w io.Writer }

func (p printer) f(format string, a ...any) { fmt.Fprintf(p.w, format, a...) }
func (p printer) section(title string)      { p.f("\n== %s ==\n", title) }
func (p printer) fail(what string, err error) {
	p.f("  ✗ %s: %s\n", what, logging.Redact(err.Error()))
}

func (p printer) counts(c collections.Counter[string], n int) {
	for _, e := range c.Top(n) {
		p.f("  %5d  %s\n", e.N, e.Key)
	}
}

func yesNo(b bool) string {
	if b {
		return "sim"
	}
	return "não"
}

func check(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func (r ICalReport) Print(w io.Writer, loc *time.Location, maxRows int) {
	p := printer{w}
	ts := func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.In(loc).Format("02/01/2006 15:04 MST")
	}

	p.section("iCal")
	p.f("  eventos válidos:   %d\n", r.Total)
	p.f("  eventos inválidos: %d\n", r.Invalid)
	for _, s := range r.InvalidSamples {
		p.f("    - %s\n", s)
	}
	p.f("  dia inteiro:       %d\n", r.AllDay)
	p.f("  duração zero:      %d\n", r.ZeroDuration)
	p.f("  intervalo:         %s → %s\n", ts(r.First), ts(r.Last))

	p.section("UIDs")
	p.counts(r.UIDShapes, 5)
	p.f("  exemplos: %s\n", strings.Join(r.UIDSamples, ", "))
	if n := len(r.DuplicateUIDs); n > 0 {
		p.f("  ⚠ %d UID(s) duplicado(s): %s\n", n, strings.Join(r.DuplicateUIDs[:min(maxSamples, n)], ", "))
	} else {
		p.f("  ✓ nenhum UID duplicado\n")
	}

	p.section("Propriedades iCal")
	p.counts(r.Props, 0)

	p.section("Disciplinas (CATEGORIES)")
	p.counts(r.Categories, 0)
	if r.NoCategory > 0 {
		p.f("  ⚠ %d evento(s) sem CATEGORIES\n", r.NoCategory)
	}

	p.section("Regras de título")
	p.counts(r.Rules, 0)
	p.f("  tipos:\n")
	p.counts(r.Types, 0)
	p.f("  fases:\n")
	p.counts(r.Phases, 0)
	if len(r.Unmatched) > 0 {
		p.f("  títulos sem template:\n")
		for _, s := range r.Unmatched {
			p.f("    - %s\n", s)
		}
	}

	p.section(fmt.Sprintf("Próximos %d dias", int(r.Window.Hours()/24)))
	if len(r.Upcoming) == 0 {
		p.f("  (nenhum)\n")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  QUANDO\tDISCIPLINA\tTIPO\tFASE\tTÍTULO")
	for i, e := range r.Upcoming {
		if maxRows > 0 && i == maxRows {
			fmt.Fprintf(tw, "  … +%d\t\t\t\t\n", len(r.Upcoming)-i)
			break
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
			e.At.In(loc).Format("02/01 15:04"), e.Course, e.Name.Type, e.Name.Phase, e.Name.Title)
	}
	_ = tw.Flush()
}

func (r WebServiceReport) Print(w io.Writer) {
	p := printer{w}
	p.section("Web services (" + r.BaseURL + ")")

	if pc := r.PublicConfig; pc.Err != nil {
		p.fail("config pública", pc.Err)
	} else {
		c := pc.Value
		p.f("  site:              %s\n", c.SiteName)
		p.f("  idioma padrão:     %s\n", c.Lang)
		p.f("  web services:      %s\n", yesNo(c.EnableWebServices == 1))
		p.f("  serviço mobile:    %s\n", yesNo(c.EnableMobileService == 1))
		p.f("  login do app:      %s\n", c.TypeOfLogin)
		for _, idp := range c.IdentityProviders {
			p.f("  provedor SSO:      %s\n", idp.Name)
		}
		if c.MaintenanceEnabled == 1 {
			p.f("  ⚠ site em manutenção\n")
		}
	}

	if tp := r.TokenProbe; tp.Ran {
		switch {
		case tp.Err != nil:
			p.fail("/login/token.php", tp.Err)
		case tp.Value.Available:
			p.f("  ✓ /login/token.php ativo\n")
		default:
			p.f("  ✗ /login/token.php bloqueado: %s (%s)\n", tp.Value.Message, tp.Value.ErrorCode)
		}
	}

	if !r.Site.Ran {
		p.f("  (sem token: use -user ou PRAZO_MOODLE_TOKEN para as verificações autenticadas)\n")
		return
	}
	if r.Site.Err != nil {
		p.fail("site info", r.Site.Err)
		return
	}
	si := r.Site.Value
	p.f("  moodle:            %s (versão %s)\n", si.Release, si.Version)
	p.f("  funções liberadas: %d\n", len(si.Functions))
	for _, fn := range r.Functions {
		p.f("    %s %s\n", check(fn.Available), fn.Name)
	}

	if c := r.Courses; c.Ran {
		if c.Err != nil {
			p.fail("disciplinas", c.Err)
		} else {
			p.f("\n  disciplinas matriculadas: %d\n", len(c.Value))
			for _, course := range c.Value {
				p.f("    [%s] %s (%s)\n", course.Code, course.FullName, course.ShortName)
			}
		}
	}

	if pe := r.Pending; pe.Ran {
		if pe.Err != nil {
			p.fail("eventos pendentes", pe.Err)
			return
		}
		p.f("\n  eventos pendentes (ainda não entregues): %d\n", len(pe.Value))
		for _, e := range pe.Value {
			p.f("    id=%d mod=%s curso=%s nome=%q\n", e.ID, e.ModuleName, e.Course.ShortName, e.Name)
		}
	}
}
