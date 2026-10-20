package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"prazo/internal/domain"
)

var weekdays = [...]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}

func printDeadlines(w io.Writer, ds []domain.Deadline, now time.Time, loc *time.Location, withLinks bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := "QUANDO\tFALTA\tTÍTULO"
	if withLinks {
		header += "\tLINK"
	}
	fmt.Fprintln(tw, header)
	for _, d := range ds {
		at := d.StartsAt.In(loc)
		row := fmt.Sprintf("%s %s\t%s\t%s", weekdays[at.Weekday()], at.Format("02/01 15:04"), timeLeft(now, d.StartsAt), d.DisplayTitle())
		if withLinks {
			row += "\t" + d.URL
		}
		fmt.Fprintln(tw, row)
	}
	_ = tw.Flush()
}

// timeLeft formata o tempo restante: "45min", "5h", "2d 4h" ou "vencido".
func timeLeft(now, t time.Time) string {
	d := t.Sub(now)
	hours := int(d.Hours())
	switch {
	case d < 0:
		return "vencido"
	case d < time.Hour:
		return fmt.Sprintf("%dmin", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", hours)
	case hours%24 == 0:
		return fmt.Sprintf("%dd", hours/24)
	default:
		return fmt.Sprintf("%dd %dh", hours/24, hours%24)
	}
}
