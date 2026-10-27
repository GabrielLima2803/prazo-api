package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	MasterKeySize = 32
	version       = 1
)

var (
	ErrKeySize    = fmt.Errorf("a chave precisa ter %d bytes", MasterKeySize)
	ErrCiphertext = errors.New("texto cifrado inválido ou adulterado")
)

type Sealer struct {
	aead cipher.AEAD
}

func NewSealer(key []byte) (*Sealer, error) {
	if len(key) != MasterKeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// ParseKey decodifica uma chave em base64 (padrão ou URL-safe).
func ParseKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		if k, err := enc.DecodeString(s); err == nil {
			if len(k) != MasterKeySize {
				return nil, ErrKeySize
			}
			return k, nil
		}
	}
	return nil, errors.New("a chave precisa estar em base64")
}

// GenerateKey devolve uma chave nova em base64.
func GenerateKey() string {
	k := make([]byte, MasterKeySize)
	rand.Read(k)
	return base64.StdEncoding.EncodeToString(k)
}

// Seal cifra plaintext. aad (associated data) amarra o texto cifrado ao seu
// contexto (ex.: o nome do segredo), impedindo que seja trocado de lugar.
func (s *Sealer) Seal(plaintext, aad []byte) []byte {
	out := make([]byte, 1+s.aead.NonceSize(), 1+s.aead.NonceSize()+len(plaintext)+s.aead.Overhead())
	out[0] = version
	rand.Read(out[1:])
	return s.aead.Seal(out, out[1:], plaintext, aad)
}

func (s *Sealer) Open(ciphertext, aad []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(ciphertext) < 1+n+s.aead.Overhead() || ciphertext[0] != version {
		return nil, ErrCiphertext
	}
	pt, err := s.aead.Open(nil, ciphertext[1:1+n], ciphertext[1+n:], aad)
	if err != nil {
		return nil, ErrCiphertext
	}
	return pt, nil
}
