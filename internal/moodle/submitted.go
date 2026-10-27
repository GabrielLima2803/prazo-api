package moodle

import (
	"github.com/GabrielLima2803/prazo-sync/internal/storage"

	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/collections"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

const pendingPages = 6

// markSubmitted marca Submitted nos prazos já entregues.
//
// O Moodle só lista o que ainda exige ação do aluno
// (core_calendar_get_action_events_by_timesort). "Não está na lista" sozinho
// não basta: atividades sem envio pelo Moodle nunca aparecem. Por isso só vira
// entregue o que já apareceu como pendente antes e saiu da lista antes do
// prazo. Sem token ou com falha nos web services, nada é marcado.
func (s *AccountService) markSubmitted(ctx context.Context, m Account, deadlines []domain.Deadline) {
	if m.Token == "" || s.kv == nil {
		return
	}
	now := s.now()
	events, err := NewWebServiceClient(m.BaseURL, s.http).AllPendingEvents(ctx, m.Token, now.Add(-24*time.Hour), pendingPages)
	if err != nil {
		s.log.Debug("status de entrega indisponível", "err", err)
		return
	}
	pending := collections.Set[int64]{}
	for _, e := range events {
		pending.Add(e.ID)
	}
	seen := s.loadSeen(ctx)
	for id := range pending {
		seen.Add(id)
	}
	for i, d := range deadlines {
		id := eventID(d.SourceID)
		if id != 0 && seen.Has(id) && !pending.Has(id) && d.EndsAt.After(now) {
			deadlines[i].Submitted = true
		}
	}
	s.saveSeen(ctx, seen)
}

// eventID extrai o id numérico do UID do iCal ("123@host"), o mesmo dos web services.
func eventID(sourceID string) int64 {
	n, _ := strconv.ParseInt(strings.SplitN(sourceID, "@", 2)[0], 10, 64)
	return n
}

func (s *AccountService) loadSeen(ctx context.Context) collections.Set[int64] {
	seen := collections.Set[int64]{}
	b, err := s.kv.Get(ctx, storage.KeyPendingSeen)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			s.log.Warn("ler pendências vistas", "err", err)
		}
		return seen
	}
	var ids []int64
	if json.Unmarshal(b, &ids) == nil {
		for _, id := range ids {
			seen.Add(id)
		}
	}
	return seen
}

func (s *AccountService) saveSeen(ctx context.Context, seen collections.Set[int64]) {
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	b, _ := json.Marshal(ids)
	if err := s.kv.Put(ctx, storage.KeyPendingSeen, b); err != nil {
		s.log.Warn("guardar pendências vistas", "err", err)
	}
}
