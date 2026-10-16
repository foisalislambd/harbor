package id

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func New(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(buf)
}

func Token(prefix string) (raw, hash, displayPrefix string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	raw = prefix + base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	n := 12
	if len(raw) < n {
		n = len(raw)
	}
	displayPrefix = raw[:n]
	return raw, hash, displayPrefix, nil
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func Secret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func Slug(name string) string {
	var b []byte
	dash := false
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			b = append(b, byte(r-'A'+'a'))
			dash = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b = append(b, byte(r))
			dash = false
		default:
			if !dash && len(b) > 0 {
				b = append(b, '-')
				dash = true
			}
		}
	}
	for len(b) > 0 && b[len(b)-1] == '-' {
		b = b[:len(b)-1]
	}
	if len(b) > 48 {
		b = b[:48]
		for len(b) > 0 && b[len(b)-1] == '-' {
			b = b[:len(b)-1]
		}
	}
	if len(b) == 0 {
		return "project"
	}
	return string(b)
}

func UniqueSlug(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for i := 2; i < 10000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !taken(candidate) {
			return candidate
		}
	}
	return base + "-" + New("")
}
