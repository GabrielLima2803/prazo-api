package doctor

import (
	"github.com/GabrielLima2803/prazo-sync/internal/moodle"

	"cmp"
	"regexp"
	"slices"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/collections"
)

const (
	maxSamples   = 5
	maxUnmatched = 15
	noRule       = "(nenhuma)"
)

var digits = regexp.MustCompile(`\d+`)

// AnalyzeICal resume o feed. Eventos em [now, now+window) entram em Upcoming.
func AnalyzeICal(feed moodle.Feed, now time.Time, window time.Duration) ICalReport {
	r := ICalReport{
		Total:      len(feed.Events),
		Invalid:    len(feed.Invalid),
		UIDShapes:  collections.Counter[string]{},
		Props:      collections.Counter[string]{},
		Categories: collections.Counter[string]{},
		Rules:      collections.Counter[string]{},
		Types:      collections.Counter[string]{},
		Phases:     collections.Counter[string]{},
		Window:     window,
	}
	for _, err := range feed.Invalid[:min(maxSamples, len(feed.Invalid))] {
		r.InvalidSamples = append(r.InvalidSamples, err.Error())
	}

	seenUIDs := collections.Set[string]{}
	unmatched := collections.Set[string]{}
	for _, e := range feed.Events {
		if seenUIDs.Has(e.UID) {
			r.DuplicateUIDs = append(r.DuplicateUIDs, e.UID)
		}
		seenUIDs.Add(e.UID)

		shape := digits.ReplaceAllString(e.UID, "#")
		if r.UIDShapes[shape] == 0 && len(r.UIDSamples) < 3 {
			r.UIDSamples = append(r.UIDSamples, e.UID)
		}
		r.UIDShapes.Add(shape)

		for _, p := range e.Props {
			r.Props.Add(p)
		}
		if len(e.Categories) == 0 {
			r.NoCategory++
		}
		for _, c := range e.Categories {
			r.Categories.Add(c)
		}

		name := moodle.ParseEventName(e.Summary)
		rule := cmp.Or(name.Rule, noRule)
		if rule == noRule && !unmatched.Has(e.Summary) && unmatched.Len() < maxUnmatched {
			unmatched.Add(e.Summary)
			r.Unmatched = append(r.Unmatched, e.Summary)
		}
		r.Rules.Add(rule)
		r.Types.Add(string(name.Type))
		r.Phases.Add(cmp.Or(string(name.Phase), noRule))

		if e.AllDay {
			r.AllDay++
		}
		if e.End.Equal(e.Start) {
			r.ZeroDuration++
		}
		if r.First.IsZero() || e.Start.Before(r.First) {
			r.First = e.Start
		}
		if e.Start.After(r.Last) {
			r.Last = e.Start
		}
		if !e.Start.Before(now) && e.Start.Before(now.Add(window)) {
			course := ""
			if len(e.Categories) > 0 {
				course = e.Categories[0]
			}
			r.Upcoming = append(r.Upcoming, UpcomingEvent{At: e.Start, Course: course, Name: name})
		}
	}
	slices.SortFunc(r.Upcoming, func(a, b UpcomingEvent) int { return a.At.Compare(b.At) })
	return r
}
