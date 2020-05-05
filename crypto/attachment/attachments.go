// Copyright (c) 2020 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package attachment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

var (
	HashMismatch         = errors.New("mismatching SHA-256 digest")
	UnsupportedVersion   = errors.New("unsupported Matrix file encryption version")
	UnsupportedAlgorithm = errors.New("unsupported JWK encryption algorithm")
	InvalidKey           = errors.New("failed to decode key")
	InvalidInitVector    = errors.New("failed to decode initialization vector")
)

const (
	keyLength  = 32
	hashLength = 32
	ivLength   = 16
)

var (
	keyBase64Length  = base64.RawURLEncoding.EncodedLen(keyLength)
	hashBase64Length = base64.RawStdEncoding.EncodedLen(hashLength)
	ivBase64Length   = base64.RawStdEncoding.EncodedLen(ivLength)
)

type JSONWebKey struct {
	Key         string   `json:"k"`
	Algorithm   string   `json:"alg"`
	Extractable bool     `json:"ext"`
	KeyType     string   `json:"kty"`
	KeyOps      []string `json:"key_ops"`
}

type EncryptedFileHashes struct {
	SHA256 string `json:"sha256"`
}

type decodedKeys struct {
	key [keyLength]byte
	iv  [ivLength]byte
}

type EncryptedFile struct {
	Key        JSONWebKey          `json:"key"`
	InitVector string              `json:"iv"`
	Hashes     EncryptedFileHashes `json:"hashes"`
	Version    string              `json:"v"`

	decoded *decodedKeys `json:"-"`
}

func NewEncryptedFile() *EncryptedFile {
	key, iv := genA256CTR()
	return &EncryptedFile{
		Key: JSONWebKey{
			Key:         base64.RawURLEncoding.EncodeToString(key[:]),
			Algorithm:   "A256CTR",
			Extractable: true,
			KeyType:     "oct",
			KeyOps:      []string{"encrypt", "decrypt"},
		},
		InitVector: base64.RawStdEncoding.EncodeToString(iv[:]),
		Version:    "v2",

		decoded: &decodedKeys{key, iv},
	}
}


func (ef *EncryptedFile) decodeKeys() error {
	if ef.decoded != nil {
		return nil
	} else if len(ef.Key.Key) != keyBase64Length {
		return InvalidKey
	} else if len(ef.InitVector) != ivBase64Length {
		return InvalidInitVector
	}
	ef.decoded = &decodedKeys{}
	_, err := base64.RawURLEncoding.Decode(ef.decoded.key[:], []byte(ef.Key.Key))
	if err != nil {
		return InvalidKey
	}
	_, err = base64.RawStdEncoding.Decode(ef.decoded.iv[:], []byte(ef.InitVector))
	if err != nil {
		return InvalidInitVector
	}
	return nil
}

func (ef *EncryptedFile) Encrypt(plaintext []byte) []byte {
	ef.decodeKeys()
	ciphertext := xorA256CTR(plaintext, ef.decoded.key, ef.decoded.iv)
	hash := sha256.Sum256(ciphertext)
	ef.Hashes.SHA256 = base64.RawStdEncoding.EncodeToString(hash[:])
	return ciphertext
}

func (ef *EncryptedFile) checkHash(ciphertext []byte) bool {
	if len(ef.Hashes.SHA256) != hashBase64Length {
		return false
	}
	var hash [hashLength]byte
	_, err := base64.RawStdEncoding.Decode(hash[:], []byte(ef.Hashes.SHA256))
	if err != nil {
		return false
	}
	return hash == sha256.Sum256(ciphertext)
}

func (ef *EncryptedFile) Decrypt(ciphertext []byte) ([]byte, error) {
	if ef.Version != "v2" {
		return nil, UnsupportedVersion
	} else if ef.Key.Algorithm != "A256CTR" {
		return nil, UnsupportedAlgorithm
	} else if !ef.checkHash(ciphertext) {
		return nil, HashMismatch
	} else if err := ef.decodeKeys(); err != nil {
		return nil, err
	} else {
		return xorA256CTR(ciphertext, ef.decoded.key, ef.decoded.iv), nil
	}
}

func xorA256CTR(source []byte, key [keyLength]byte, iv [ivLength]byte) []byte {
	block, _ := aes.NewCipher(key[:])
	result := make([]byte, len(source))
	cipher.NewCTR(block, iv[:]).XORKeyStream(result, source)
	return result
}

func genA256CTR() (key [keyLength]byte, iv [ivLength]byte) {
	_, err := rand.Read(key[:])
	if err != nil {
		panic(err)
	}

	// For some reason we leave the 8 last bytes empty even though AES256-CTR has a 16-byte block size.
	_, err = rand.Read(iv[:8])
	if err != nil {
		panic(err)
	}
	return
}
