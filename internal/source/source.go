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
