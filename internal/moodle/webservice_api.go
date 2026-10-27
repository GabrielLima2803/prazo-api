package moodle

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

func (c *WebServiceClient) SiteInfo(ctx context.Context, token string) (SiteInfo, error) {
	var si SiteInfo
	err := c.Call(ctx, token, "core_webservice_get_site_info", nil, &si)
	return si, err
}

func (c *WebServiceClient) UserCourses(ctx context.Context, token string, userID int64) ([]Course, error) {
	var courses []Course
	args := url.Values{"userid": {strconv.FormatInt(userID, 10)}}
	err := c.Call(ctx, token, "core_enrol_get_users_courses", args, &courses)
	return courses, err
}

// PendingEvents lista eventos a partir de from que ainda exigem ação do aluno.
// O id de cada evento é o mesmo número do UID no iCal.
func (c *WebServiceClient) PendingEvents(ctx context.Context, token string, from time.Time, limit int) ([]ActionEvent, error) {
	return c.pendingPage(ctx, token, from, limit, 0)
}

// AllPendingEvents pagina PendingEvents (o Moodle devolve no máximo 50 por
// vez) até maxPages.
func (c *WebServiceClient) AllPendingEvents(ctx context.Context, token string, from time.Time, maxPages int) ([]ActionEvent, error) {
	const page = 50
	var all []ActionEvent
	var after int64
	for range maxPages {
		events, err := c.pendingPage(ctx, token, from, page, after)
		if err != nil {
			return nil, err
		}
		all = append(all, events...)
		if len(events) < page {
			break
		}
		after = events[len(events)-1].ID
	}
	return all, nil
}

func (c *WebServiceClient) pendingPage(ctx context.Context, token string, from time.Time, limit int, afterID int64) ([]ActionEvent, error) {
	var resp struct {
		Events []ActionEvent `json:"events"`
	}
	args := url.Values{
		"timesortfrom": {strconv.FormatInt(from.Unix(), 10)},
		"limitnum":     {strconv.Itoa(limit)},
	}
	if afterID > 0 {
		args.Set("aftereventid", strconv.FormatInt(afterID, 10))
	}
	err := c.Call(ctx, token, "core_calendar_get_action_events_by_timesort", args, &resp)
	return resp.Events, err
}

// CalendarExportToken devolve o authtoken da exportação iCal do usuário, o
// mesmo que o Moodle põe na URL de "Exportar calendário".
func (c *WebServiceClient) CalendarExportToken(ctx context.Context, token string) (string, error) {
	var resp struct {
		Token string `json:"token"`
	}
	if err := c.Call(ctx, token, "core_calendar_get_calendar_export_token", nil, &resp); err != nil {
		return "", err
	}
	if resp.Token == "" {
		return "", ErrUnexpectedResponse
	}
	return resp.Token, nil
}
