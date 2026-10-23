// Package prefs guarda as personalizações feitas pelo app: disciplinas e
// tipos ocultos, cor por disciplina e lembretes por tipo. Elas se somam aos
// filtros das variáveis de ambiente.
package prefs

import (
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

type Preferences struct {
	HiddenCourses []string                      `json:"hiddenCourses"`
	HiddenTypes   []domain.DeadlineType         `json:"hiddenTypes"`
	CourseColors  map[string]string             `json:"courseColors"` // sigla → colorId 1–11
	Reminders     map[domain.DeadlineType][]int `json:"reminders"`    // minutos antes do prazo
}

type Store struct {
	kv store.KV
}

// Limites do Google Agenda para lembretes.
const (
	MaxReminders       = 5
	MaxReminderMinutes = 4 * 7 * 24 * 60
)
