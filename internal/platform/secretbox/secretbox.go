// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package secretbox encrypts the secrets the server must be able to read
// back (round 51, docs/24-hosztolt-ajanlat.md, section 5): the operators'
// TOTP secrets now, the workspaces' API and repository keys with the
// hosted guard. AES-256-GCM, a fresh random nonce per value, and
// associated data that binds each value to its row (table, row, workspace,
// kind), so a ciphertext copied into another row or workspace no longer
// opens.
//
// The keys come from COLONY_SECRET_KEYS ("k2:<base64>,k1:<base64>"): the
// first is the active one, the rest only decrypt (rotation). They are
// never stored in the database, so a database backup alone opens nothing.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// KeySize is the length of a key in bytes (AES-256).
const KeySize = 32

// ErrNoKey: the value was sealed with a key the keyring does not have.
var ErrNoKey = errors.New("secretbox: unknown key")

// ErrOpen: the value does not open (wrong key, wrong associated data or a
// changed ciphertext). It never says which.
var ErrOpen = errors.New("secretbox: the value does not open")

// Sealed is an encrypted value as it is stored.
type Sealed struct {
	KeyID      string
	Nonce      []byte
	Ciphertext []byte
}

// Keyring holds the active key and the older ones.
type Keyring struct {
	active string
	order  []string
	keys   map[string]cipher.AEAD
	// Rand is the nonce source; crypto/rand when nil (tests may fix it).
	Rand io.Reader
}

var keyIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Parse reads "id:<base64 of 32 bytes>[,id:…]". The first key is active.
func Parse(s string) (*Keyring, error) {
	k := &Keyring{keys: map[string]cipher.AEAD{}}
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, b64, ok := strings.Cut(part, ":")
		if !ok || !keyIDRe.MatchString(id) {
			return nil, fmt.Errorf("secretbox: a key must look like id:<base64>, with a short lower-case id")
		}
		if _, dup := k.keys[id]; dup {
			return nil, fmt.Errorf("secretbox: key id %q appears twice", id)
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(raw) != KeySize {
			return nil, fmt.Errorf("secretbox: key %q must be %d bytes in base64", id, KeySize)
		}
		aead, err := newAEAD(raw)
		if err != nil {
			return nil, err
		}
		k.keys[id] = aead
		k.order = append(k.order, id)
	}
	if len(k.order) == 0 {
		return nil, errors.New("secretbox: no key")
	}
	k.active = k.order[0]
	return k, nil
}

// NewKey returns a fresh key in the COLONY_SECRET_KEYS form (id:<base64>).
func NewKey(id string) (string, error) {
	if !keyIDRe.MatchString(id) {
		return "", fmt.Errorf("secretbox: bad key id %q", id)
	}
	raw := make([]byte, KeySize)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return id + ":" + base64.StdEncoding.EncodeToString(raw), nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return cipher.NewGCM(block)
}

// Active is the id of the key new values are sealed with.
func (k *Keyring) Active() string { return k.active }

// Seal encrypts plaintext with the active key, bound to aad (see AAD).
func (k *Keyring) Seal(plaintext, aad []byte) (Sealed, error) {
	aead := k.keys[k.active]
	nonce := make([]byte, aead.NonceSize())
	r := k.Rand
	if r == nil {
		r = rand.Reader
	}
	if _, err := io.ReadFull(r, nonce); err != nil {
		return Sealed{}, fmt.Errorf("secretbox: nonce: %w", err)
	}
	return Sealed{KeyID: k.active, Nonce: nonce, Ciphertext: aead.Seal(nil, nonce, plaintext, aad)}, nil
}

// Open decrypts a sealed value; aad must be the one it was sealed with.
func (k *Keyring) Open(s Sealed, aad []byte) ([]byte, error) {
	aead, ok := k.keys[s.KeyID]
	if !ok {
		return nil, ErrNoKey
	}
	if len(s.Nonce) != aead.NonceSize() {
		return nil, ErrOpen
	}
	out, err := aead.Open(nil, s.Nonce, s.Ciphertext, aad)
	if err != nil {
		return nil, ErrOpen
	}
	return out, nil
}

// NeedsRotation reports whether s was sealed with an older key.
func (k *Keyring) NeedsRotation(s Sealed) bool { return s.KeyID != k.active }

// AAD builds the associated data from its parts (table, row id, workspace
// id, kind …), each length-prefixed so no two lists give the same bytes.
func AAD(parts ...string) []byte {
	var out []byte
	for _, p := range parts {
		out = binary.BigEndian.AppendUint32(out, uint32(len(p)))
		out = append(out, p...)
	}
	return out
}
