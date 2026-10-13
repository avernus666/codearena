package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

const passwordRounds = 120000

func hashPassword(password string) (string, string) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		panic(err)
	}
	salt := hex.EncodeToString(saltBytes)
	return salt, hex.EncodeToString(pbkdf2([]byte(password), saltBytes, passwordRounds, 32))
}
func verifyPassword(password, salt, expected string) bool {
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		return false
	}
	actual := hex.EncodeToString(pbkdf2([]byte(password), saltBytes, passwordRounds, 32))
	a, e1 := hex.DecodeString(actual)
	b, e2 := hex.DecodeString(expected)
	return e1 == nil && e2 == nil && hmac.Equal(a, b)
}
func pbkdf2(password, salt []byte, rounds, keyLen int) []byte {
	out := make([]byte, 0, keyLen)
	for block := uint32(1); len(out) < keyLen; block++ {
		m := hmac.New(sha256.New, password)
		m.Write(salt)
		m.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := m.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < rounds; i++ {
			m = hmac.New(sha256.New, password)
			m.Write(u)
			u = m.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
func (s *Server) signSession(id string) string {
	expires := time.Now().Add(14 * 24 * time.Hour).Unix()
	payload := base64.RawURLEncoding.EncodeToString([]byte(id + "." + strconv.FormatInt(expires, 10)))
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Server) verifySession(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", errors.New("invalid session")
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(parts[0]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return "", errors.New("invalid session signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	values := strings.Split(string(raw), ".")
	if len(values) != 2 {
		return "", errors.New("invalid session payload")
	}
	expiry, err := strconv.ParseInt(values[1], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return "", errors.New("session expired")
	}
	return values[0], nil
}
