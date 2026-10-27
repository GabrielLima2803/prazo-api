package api

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	l := newLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	if !l.allow("ana") || !l.allow("ana") {
		t.Fatal("as duas primeiras tentativas devem passar")
	}
	if l.allow("ana") {
		t.Fatal("a terceira tentativa na janela deve ser recusada")
	}
	if !l.allow("bia") {
		t.Fatal("o limite é por chave")
	}

	now = now.Add(time.Minute + time.Second)
	if !l.allow("ana") {
		t.Fatal("a janela vencida deve recomeçar a contagem")
	}
	if len(l.hits) != 1 {
		t.Errorf("janelas vencidas devem ser descartadas; sobraram %d", len(l.hits))
	}
}
