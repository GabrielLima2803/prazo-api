// Package source define de onde vêm os prazos (iCal, web services).
package source

import (
	"context"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

// Source devolve todos os prazos conhecidos pela fonte, sem filtros nem
// agrupamento (isso é responsabilidade de domain.Normalize).
//
// Um erro significa que a lista NÃO está completa: quem chama nunca deve
// apagar eventos com base num fetch que falhou.
type Source interface {
	Fetch(ctx context.Context) ([]domain.Deadline, error)
}

// Func adia a escolha da fonte para cada Fetch. Permite trocar a conta do
// Moodle pelo app sem reiniciar o servidor.
type Func func(ctx context.Context) (Source, error)

func (f Func) Fetch(ctx context.Context) ([]domain.Deadline, error) {
	s, err := f(ctx)
	if err != nil {
		return nil, err
	}
	return s.Fetch(ctx)
}
