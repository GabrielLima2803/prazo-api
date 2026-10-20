package domain

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"prazo/internal/collections"
)

var t0 = time.Date(2026, 10, 21, 2, 59, 0, 0, time.UTC)

func dl(id, course, title string, typ DeadlineType, phase Phase, at time.Time) Deadline {
	return Deadline{SourceID: id, CourseCode: courseCodes[course], CourseName: course, Type: typ, Phase: phase, Title: title, StartsAt: at, EndsAt: at}
}

const (
	cdi = "2026/2_CCT_CCI251-03U_CDI2002"
	poo = "2026/2_CCT_CCI251-03A_POO0002"
)

var courseCodes = map[string]string{cdi: "CDI", poo: "POO"}

func TestCollapseKeepsOnlyClose(t *testing.T) {
	in := []Deadline{
		dl("1", cdi, "TAREFA 9: Séries", TypeAssign, PhaseOpen, t0.Add(-72*time.Hour)),
		dl("2", cdi, "TAREFA 9: Séries", TypeAssign, PhaseClose, t0),
		dl("3", cdi, "TAREFA  9: séries", TypeAssign, PhaseExpect, t0), // espaço/caixa diferentes
		dl("4", cdi, "TAREFA 10: Taylor", TypeAssign, PhaseOpen, t0),   // ainda sem Término
		dl("5", cdi, "TAREFA 4: Impróprias", TypeAssign, PhaseExpect, t0),
		dl("6", "OUTRA", "TAREFA 9: Séries", TypeAssign, PhaseExpect, t0), // outra disciplina
	}
	got := ids(Normalize(in, NormalizeOptions{}))
	if want := "2,4,5,6"; got != want {
		t.Fatalf("ids = %s; quer %s", got, want)
	}

	got = ids(Normalize(in, NormalizeOptions{IncludeOpenings: true}))
	if want := "1,2,4,5,6"; got != want {
		t.Fatalf("com aberturas: ids = %s; quer %s", got, want)
	}
}

func TestExpectDroppedWhenDue(t *testing.T) {
	in := []Deadline{
		dl("1", cdi, "Lista 3", TypeAssign, PhaseDue, t0),
		dl("2", cdi, "Lista 3", TypeAssign, PhaseExpect, t0),
		dl("3", cdi, "Lista 3", TypeAssign, PhaseGrading, t0),
	}
	if got := ids(Normalize(in, NormalizeOptions{})); got != "1" {
		t.Fatalf("ids = %s; quer 1", got)
	}
}

func TestFilter(t *testing.T) {
	in := []Deadline{
		dl("1", cdi, "Lista 1", TypeAssign, PhaseDue, t0),
		dl("2", poo, "Aula Prática 9", TypeAssign, PhaseDue, t0),
		dl("3", poo, "Fórum de dúvidas", TypeForum, PhaseDue, t0),
		dl("4", poo, "Leitura opcional", TypeAssign, PhaseDue, t0),
	}
	f := Filter{
		Courses: collections.NewSet("CDI"),
		Types:   collections.NewSet(TypeForum),
		Titles:  regexp.MustCompile(`(?i)opcional`),
	}
	if got := ids(Normalize(in, NormalizeOptions{Filter: f})); got != "2" {
		t.Fatalf("ids = %s; quer 2", got)
	}
}

func TestNormalizeSortsAndHashes(t *testing.T) {
	in := []Deadline{
		dl("b", cdi, "B", TypeAssign, PhaseDue, t0.Add(time.Hour)),
		dl("a", cdi, "A", TypeAssign, PhaseDue, t0),
	}
	out := Normalize(in, NormalizeOptions{})
	if ids(out) != "a,b" {
		t.Fatalf("ordem errada: %s", ids(out))
	}
	for _, d := range out {
		if d.ContentHash == "" || d.ContentHash != d.ComputeHash() {
			t.Fatalf("hash ausente/errado em %s", d.SourceID)
		}
	}
}

func TestHashChangesWithRelevantFields(t *testing.T) {
	base := dl("1", cdi, "Lista 3", TypeAssign, PhaseDue, t0)
	h := base.ComputeHash()

	same := base
	same.SourceID = "outro" // chave, não conteúdo
	if same.ComputeHash() != h {
		t.Error("SourceID não deveria afetar o hash")
	}

	mutations := map[string]func(*Deadline){
		"prazo":     func(d *Deadline) { d.StartsAt = d.StartsAt.Add(time.Minute) },
		"título":    func(d *Deadline) { d.Title = "Lista 4" },
		"entregue":  func(d *Deadline) { d.Submitted = true },
		"descrição": func(d *Deadline) { d.Description = "nova" },
		"url":       func(d *Deadline) { d.URL = "https://x" },
	}
	for name, mut := range mutations {
		d := base
		mut(&d)
		if d.ComputeHash() == h {
			t.Errorf("mudar %s deveria mudar o hash", name)
		}
	}

	// Mesmo instante em outro fuso = mesmo hash.
	loc := time.FixedZone("-03", -3*3600)
	d := base
	d.StartsAt, d.EndsAt = d.StartsAt.In(loc), d.EndsAt.In(loc)
	if d.ComputeHash() != h {
		t.Error("fuso de exibição não deveria afetar o hash")
	}
}

func TestDisplayTitle(t *testing.T) {
	cases := []struct {
		d    Deadline
		want string
	}{
		{dl("", poo, "Aula Prática 9", TypeAssign, PhaseDue, t0), "[POO] Entrega: Aula Prática 9"},
		{dl("", poo, "Entrega Etapa 3 do Trabalho", TypeAssign, PhaseDue, t0), "[POO] Entrega Etapa 3 do Trabalho"},
		{dl("", cdi, "P1", TypeExam, PhaseDue, t0), "[CDI] Prova: P1"},
		{dl("", cdi, "TAREFA 9", TypeAssign, PhaseOpen, t0), "[CDI] Abertura: TAREFA 9"},
		{dl("", "", "Prova de ALI - P1", TypeExam, PhaseNone, t0), "Prova de ALI - P1"},
	}
	for _, c := range cases {
		if got := c.d.DisplayTitle(); got != c.want {
			t.Errorf("DisplayTitle = %q; quer %q", got, c.want)
		}
	}
	d := cases[0].d
	d.Submitted = true
	if got := d.DisplayTitle(); got != "✅ [POO] Entrega: Aula Prática 9" {
		t.Errorf("entregue: %q", got)
	}
}

func TestParseDeadlineType(t *testing.T) {
	if got, err := ParseDeadlineType(" Forum "); err != nil || got != TypeForum {
		t.Errorf("ParseDeadlineType = %q, %v", got, err)
	}
	if _, err := ParseDeadlineType("prova"); !errors.Is(err, ErrUnknownType) {
		t.Errorf("esperava ErrUnknownType, veio %v", err)
	}
}

func ids(ds []Deadline) string {
	s := ""
	for i, d := range ds {
		if i > 0 {
			s += ","
		}
		s += d.SourceID
	}
	return s
}
