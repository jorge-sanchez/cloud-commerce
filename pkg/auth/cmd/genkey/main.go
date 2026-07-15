// genkey prints a fresh Ed25519 keypair in the env-var format pkg/auth
// expects. Usage: go run ./cmd/genkey (from pkg/auth). The private seed
// belongs in Secret Manager (jwt-private-key) or a local .env — never in
// the repository.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func main() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	fmt.Printf("JWT_PRIVATE_KEY=%s\n", base64.StdEncoding.EncodeToString(priv.Seed()))
	fmt.Printf("JWT_PUBLIC_KEY=%s\n", base64.StdEncoding.EncodeToString(pub))
}
