// Package doctor diagnostica uma instância do Moodle (exportação iCal e web
// services) para calibrar as regras de normalização. Só lê.
package doctor

import (
	"time"

	"prazo/internal/collections"
	"prazo/internal/moodle"
	"prazo/internal/source/webservice"
)

// Result guarda o resultado de uma verificação que pode falhar ou não ter
// sido executada, sem interromper as demais.
type Result[T any] struct {
	Value T
	Err   error
	Ran   bool
}

func run[T any](v T, err error) Result[T] { return Result[T]{Value: v, Err: err, Ran: true} }

type ICalReport struct {
	Total          int
	Invalid        int
	InvalidSamples []string
	DuplicateUIDs  []string
	UIDShapes      collections.Counter[string]
	UIDSamples     []string
	Props          collections.Counter[string]
	Categories     collections.Counter[string]
	NoCategory     int
	Rules          collections.Counter[string]
	Types          collections.Counter[string]
	Phases         collections.Counter[string]
	Unmatched      []string
	AllDay         int
	ZeroDuration   int
	First, Last    time.Time
	Window         time.Duration
	Upcoming       []UpcomingEvent
}

type UpcomingEvent struct {
	At     time.Time
	Course string
	Name   moodle.EventName
}

type WebServiceOptions struct {
	ProbeLogin bool
	Token      string // vazio: pula as verificações autenticadas
	Now        time.Time
}

type FunctionStatus struct {
	Name      string
	Available bool
}

type CourseInfo struct {
	Code string
	webservice.Course
}

type WebServiceReport struct {
	BaseURL      string
	PublicConfig Result[webservice.PublicConfig]
	TokenProbe   Result[webservice.TokenProbe]
	Site         Result[webservice.SiteInfo]
	Functions    []FunctionStatus
	Courses      Result[[]CourseInfo]
	Pending      Result[[]webservice.ActionEvent]
}
