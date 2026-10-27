package storage

import (
	"bytes"
	"errors"
	"testing"
)

func newSealer(t *testing.T) *Sealer {
	t.Helper()
	key, err := ParseKey(GenerateKey())
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRoundTrip(t *testing.T) {
	s := newSealer(t)
	pt := []byte(`{"refresh_token":"1//abc"}`)
	ct := s.Seal(pt, []byte("google_token"))
	if bytes.Contains(ct, pt) || bytes.Contains(ct, []byte("refresh")) {
		t.Fatal("texto cifrado contém o texto plano")
	}
	got, err := s.Open(ct, []byte("google_token"))
	if err != nil || !bytes.Equal(got, pt) {
		t.Fatalf("Open = %q, %v", got, err)
	}
}

func TestNonceIsRandom(t *testing.T) {
	s := newSealer(t)
	if bytes.Equal(s.Seal([]byte("x"), nil), s.Seal([]byte("x"), nil)) {
		t.Fatal("duas cifragens do mesmo texto deram o mesmo resultado")
	}
}

func TestRejectsTampering(t *testing.T) {
	s := newSealer(t)
	ct := s.Seal([]byte("segredo"), []byte("a"))

	flipped := bytes.Clone(ct)
	flipped[len(flipped)-1] ^= 1

	for name, tc := range map[string]struct {
		ct, aad []byte
	}{
		"byte alterado": {flipped, []byte("a")},
		"aad trocado":   {ct, []byte("b")},
		"truncado":      {ct[:10], []byte("a")},
		"versão":        {append([]byte{9}, ct[1:]...), []byte("a")},
	} {
		if _, err := s.Open(tc.ct, tc.aad); !errors.Is(err, ErrCiphertext) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestWrongKey(t *testing.T) {
	ct := newSealer(t).Seal([]byte("segredo"), nil)
	if _, err := newSealer(t).Open(ct, nil); !errors.Is(err, ErrCiphertext) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseKey(t *testing.T) {
	if _, err := ParseKey("curta"); err == nil {
		t.Error("aceitou base64 inválido")
	}
	if _, err := ParseKey("c2hvcnQ="); !errors.Is(err, ErrKeySize) {
		t.Errorf("chave curta: %v", err)
	}
	if _, err := NewSealer(make([]byte, 16)); !errors.Is(err, ErrKeySize) {
		t.Errorf("NewSealer com 16 bytes: %v", err)
	}
}
