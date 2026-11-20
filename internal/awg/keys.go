package awg

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// KeyPair is one freshly generated WireGuard key pair.
type KeyPair struct {
	Private, Public string
}

// GenerateKeyPair asks awg for a key pair, falling back to one drawn from
// crypto/rand when the binary is missing - or when it produced a private key
// but could not derive the public one.
//
// The fallback derives the public half from the private one (X25519) instead
// of drawing a second random key. Two unrelated keys would write and read back
// cleanly, which is exactly the problem: the server's PrivateKey would not
// match the PublicKey its clients are handed, nothing in the panel would say
// so, and the tunnel could never handshake.
func (t *Tools) GenerateKeyPair() KeyPair {
	priv, err := t.run.Run("awg genkey")
	if err == nil {
		if pub, err := t.run.Run(fmt.Sprintf("echo '%s' | awg pubkey", priv)); err == nil {
			return KeyPair{Private: priv, Public: pub}
		}
	}
	return randomKeyPair()
}

// randomKeyPair draws a Curve25519 private key and computes its public key.
func randomKeyPair() KeyPair {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return KeyPair{
		Private: base64.StdEncoding.EncodeToString(key.Bytes()),
		Public:  base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()),
	}
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
