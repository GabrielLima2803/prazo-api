package sync

import (
	"context"
	"errors"
	"log/slog"
	gosync "sync"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/source"
)

var (
	ErrBusy   = errors.New("já existe uma sincronização em andamento")
	ErrClosed = errors.New("conta excluída; a sincronização foi desligada")
)

// TargetFunc monta o destino a cada execução. Assim um "prazo auth" feito com
// o servidor no ar passa a valer sem reiniciar.
type TargetFunc func(ctx context.Context) (Target, error)

// Run é o resultado de uma execução, exposto pela API.
type Run struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Report     Report
	Err        error
}

// Runner serializa as sincronizações (agendadas ou pedidas pelo app) e
// guarda o último resultado e o último plano lido.
type Runner struct {
	svc    *Service
	target TargetFunc
	log    *slog.Logger
	now    func() time.Time

	running gosync.Mutex
	closed  bool // protegido por running

	mu      gosync.RWMutex
	last    *Run
	plan    *Plan
	nextRun time.Time
}

func NewRunner(svc *Service, target TargetFunc, log *slog.Logger) *Runner {
	return &Runner{svc: svc, target: target, log: log, now: svc.now}
}

// RunNow sincroniza agora, ou devolve ErrBusy se outra execução estiver em
// andamento.
func (r *Runner) RunNow(ctx context.Context) (Run, error) {
	if !r.running.TryLock() {
		return Run{}, ErrBusy
	}
	defer r.running.Unlock()
	if r.closed {
		return Run{}, ErrClosed
	}

	run := Run{StartedAt: r.now().UTC()}
	t, err := r.target(ctx)
	if err == nil {
		run.Report, err = r.svc.Sync(ctx, t)
	}
	run.FinishedAt = r.now().UTC()
	run.Err = err

	r.mu.Lock()
	r.last = &run
	if err == nil || len(run.Report.Plan.Deadlines) > 0 {
		r.plan = &run.Report.Plan
	}
	r.mu.Unlock()

	attrs := []any{"criados", run.Report.Created, "atualizados", run.Report.Updated, "ocultos", run.Report.Hidden,
		"falhas", run.Report.Failed, "duracao", run.FinishedAt.Sub(run.StartedAt).Round(time.Millisecond)}
	if err != nil {
		r.log.Error("sync falhou", append(attrs, "err", err)...)
	} else {
		r.log.Info("sync concluído", attrs...)
	}
	return run, err
}

// Close espera a sincronização em andamento terminar, roda fn com o Runner
// travado e o desliga para sempre: nenhuma execução posterior grava nada.
// Usado ao excluir a conta, para que uma sincronização atrasada não recrie
// os dados apagados.
func (r *Runner) Close(fn func() error) error {
	r.running.Lock()
	defer r.running.Unlock()
	r.closed = true
	return fn()
}

// Invalidate descarta o plano em cache, por exemplo depois de trocar a conta
// do Moodle.
func (r *Runner) Invalidate() {
	r.mu.Lock()
	r.plan = nil
	r.mu.Unlock()
}

func (r *Runner) Last() (Run, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.last == nil {
		return Run{}, false
	}
	return *r.last, true
}

// SetNextRun informa quando será a próxima execução agendada (quem agenda é
// o tenant.Manager, para todos os usuários).
func (r *Runner) SetNextRun(t time.Time) {
	r.mu.Lock()
	r.nextRun = t
	r.mu.Unlock()
}

func (r *Runner) NextRun() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nextRun
}

// Plan devolve o último plano se tiver menos de maxAge; senão lê a fonte de
// novo. Evita uma ida ao Moodle a cada vez que o app abre.
func (r *Runner) Plan(ctx context.Context, maxAge time.Duration) (Plan, error) {
	r.mu.RLock()
	cached := r.plan
	r.mu.RUnlock()
	if cached != nil && r.now().Sub(cached.At) < maxAge {
		return *cached, nil
	}
	p, err := r.svc.Plan(ctx)
	if errors.Is(err, source.ErrUnauthorized) {
		// Mostrar o plano antigo esconderia que nada mais está sincronizando.
		r.Invalidate()
		return Plan{}, err
	}
	if err != nil {
		if cached != nil {
			r.log.Warn("fonte indisponível; usando o último plano", "err", err)
			return *cached, nil
		}
		return Plan{}, err
	}
	r.mu.Lock()
	r.plan = &p
	r.mu.Unlock()
	return p, nil
}
