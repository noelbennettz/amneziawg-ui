package awg

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// KeyPair is one freshly generated WireGuard key pair.
type KeyPair struct {
	Private, Public string
}

// GenerateKeyPair asks awg for a key pair. Without the binary it falls back
// to two unrelated random keys: the config can still be written and read
// back, which is what a development tree without amneziawg-tools needs, but
// no tunnel will come up on it.
func (t *Tools) GenerateKeyPair() KeyPair {
	priv, err := t.run.Run("awg genkey")
	if err != nil {
		return KeyPair{Private: RandomKey(), Public: RandomKey()}
	}
	pub, err := t.run.Run(fmt.Sprintf("echo '%s' | awg pubkey", priv))
	if err != nil {
		return KeyPair{Private: RandomKey(), Public: RandomKey()}
	}
	return KeyPair{Private: priv, Public: pub}
}

// GeneratePresharedKey asks awg for a preshared key, falling back to a
// random one - which is all a preshared key is.
func (t *Tools) GeneratePresharedKey() string {
	key, err := t.run.Run("awg genpsk")
	if err != nil {
		return RandomKey()
	}
	return key
}

// RandomKey is 32 bytes from the system's cryptographic source, base64
// encoded: the shape of every WireGuard key, and what the header protection
// key is made of.
func RandomKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(b)
}
