package moodle

import (
	"context"
	"errors"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

// ErrUnauthorized indica que a fonte recusou a credencial (no Moodle: a
// senha mudou e a URL do iCal deixou de valer). Diferente de uma falha
// passageira, tentar de novo não adianta: o usuário precisa entrar de novo.
var ErrUnauthorized = errors.New("a fonte recusou a credencial")

// Source devolve todos os prazos conhecidos pela fonte, sem filtros nem
// agrupamento (isso é responsabilidade de domain.Normalize).
//
// Um erro significa que a lista NÃO está completa: quem chama nunca deve
// apagar eventos com base num fetch que falhou.
type Source interface {
	Fetch(ctx context.Context) ([]domain.Deadline, error)
}

// SourceFunc adia a escolha da fonte para cada Fetch. Permite trocar a conta do
// Moodle pelo app sem reiniciar o servidor.
type SourceFunc func(ctx context.Context) (Source, error)

func (f SourceFunc) Fetch(ctx context.Context) ([]domain.Deadline, error) {
	s, err := f(ctx)
	if err != nil {
		return nil, err
	}
	return s.Fetch(ctx)
}
