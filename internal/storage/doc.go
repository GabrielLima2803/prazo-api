// Package storage guarda o estado do Prazo sem banco de dados: chaves
// pequenas em arquivos cifrados com AES-256-GCM (um por chave, 0600), com um
// namespace por usuário. KV é o ponto de troca para um banco no futuro.
package storage
