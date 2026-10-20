package doctor

import (
	"context"

	"prazo/internal/moodle"
	"prazo/internal/source/webservice"
)

const pendingLimit = 50

// InspectWebService roda as verificações em sequência; uma falha não impede
// as seguintes, exceto quando a próxima depende do resultado anterior.
func InspectWebService(ctx context.Context, ws *webservice.Client, opts WebServiceOptions) WebServiceReport {
	r := WebServiceReport{BaseURL: ws.BaseURL()}
	r.PublicConfig = run(ws.PublicConfig(ctx))
	if opts.ProbeLogin {
		r.TokenProbe = run(ws.ProbeTokenEndpoint(ctx))
	}
	if opts.Token == "" {
		return r
	}

	r.Site = run(ws.SiteInfo(ctx, opts.Token))
	if r.Site.Err != nil {
		return r
	}
	available := r.Site.Value.FunctionSet()
	for _, fn := range webservice.RequiredFunctions {
		r.Functions = append(r.Functions, FunctionStatus{Name: fn, Available: available.Has(fn)})
	}

	if available.Has("core_enrol_get_users_courses") {
		courses, err := ws.UserCourses(ctx, opts.Token, r.Site.Value.UserID)
		infos := make([]CourseInfo, 0, len(courses))
		for _, c := range courses {
			infos = append(infos, CourseInfo{Code: moodle.CourseCode(c.ShortName), Course: c})
		}
		r.Courses = run(infos, err)
	}
	if available.Has("core_calendar_get_action_events_by_timesort") {
		r.Pending = run(ws.PendingEvents(ctx, opts.Token, opts.Now, pendingLimit))
	}
	return r
}
