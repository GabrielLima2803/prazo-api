package collections

import "testing"

func TestSet(t *testing.T) {
	s := NewSet("a", "b", "a")
	if s.Len() != 2 || !s.Has("a") || s.Has("c") {
		t.Fatalf("set = %v", s)
	}
}

func TestCounterTopIsDeterministic(t *testing.T) {
	c := Counter[string]{}
	for _, k := range []string{"b", "a", "c", "a", "b", "a"} {
		c.Add(k)
	}
	got := c.Top(2)
	if len(got) != 2 || got[0] != (Count[string]{"a", 3}) || got[1] != (Count[string]{"b", 2}) {
		t.Fatalf("Top(2) = %v", got)
	}
	if len(c.Top(0)) != 3 {
		t.Fatal("Top(0) deveria devolver todas as chaves")
	}
}
