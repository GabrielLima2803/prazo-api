package sync

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"prazo/internal/domain"
)

var weekdays = [...]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}

// Window devolve os prazos com StartsAt em [from, to). A entrada já vem
// ordenada por data (domain.Normalize).
func Window(ds []domain.Deadline, from, to time.Time) []domain.Deadline {
	var out []domain.Deadline
	for _, d := range ds {
		if !d.StartsAt.Before(from) && d.StartsAt.Before(to) {
			out = append(out, d)
		}
	}
	return out
}

// PrintTable escreve a tabela do dry-run. loc é só para exibição.
func PrintTable(w io.Writer, ds []domain.Deadline, now time.Time, loc *time.Location, withLinks bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := "QUANDO\tFALTA\tTÍTULO"
	if withLinks {
		header += "\tLINK"
	}
	fmt.Fprintln(tw, header)
	for _, d := range ds {
		at := d.StartsAt.In(loc)
		row := fmt.Sprintf("%s %s\t%s\t%s", weekdays[at.Weekday()], at.Format("02/01 15:04"), Until(now, d.StartsAt), domain.RenderTitle(d))
		if withLinks {
			row += "\t" + d.URL
		}
		fmt.Fprintln(tw, row)
	}
	_ = tw.Flush()
}

// Until formata quanto falta de forma curta: "45min", "5h", "2d 4h".
func Until(now, t time.Time) string {
	d := t.Sub(now)
	switch {
	case d < 0:
		return "vencido"
	case d < time.Hour:
		return fmt.Sprintf("%dmin", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		days := int(d.Hours()) / 24
		if h := int(d.Hours()) % 24; h > 0 {
			return fmt.Sprintf("%dd %dh", days, h)
		}
		return fmt.Sprintf("%dd", days)
	}
}
