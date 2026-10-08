package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// keyEnv names the variable holding the private key, base64 of the 64-byte
// Ed25519 key that keygen prints
const keyEnv = "MIMIC_UPDATE_PRIVATE_KEY"

func keygenCmd() error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	fmt.Println("private (keep secret, store as the", keyEnv, "secret):", base64.StdEncoding.EncodeToString(priv))
	fmt.Println("public (stamped into releases as the update key):", base64.StdEncoding.EncodeToString(pub))
	return nil
}

func signCmd(files []string) error {
	if len(files) == 0 {
		return errNoFiles
	}
	raw := os.Getenv(keyEnv)
	if raw == "" {
		return errors.New(keyEnv + " is not set")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return errors.New(keyEnv + " is not a base64 Ed25519 private key")
	}
	for _, f := range files {
		sig, err := sign(ed25519.PrivateKey(key), f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(f+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// sign signs the SHA-256 of the file. The updater's ed25519 check verifies the
// signature against the digest it computed while downloading, not the file
func sign(key ed25519.PrivateKey, path string) ([]byte, error) {
	d, err := digest(path)
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(key, d), nil
}
