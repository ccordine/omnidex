package cognitiongauntlet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const credentialedArtifactSchemaV1 = "omnidex.credentialed-artifact.v1"

type credentialedArtifact struct {
	Schema     string `json:"schema"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func sealCredentialedJSON(path string, value any, credential string, label string) error {
	if err := requireExact(credential, "credentialed artifact key", 512); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", label, err)
	}
	aead, err := credentialAEAD(credential)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("create %s nonce: %w", label, err)
	}
	envelope := credentialedArtifact{
		Schema: credentialedArtifactSchemaV1, Nonce: nonce,
		Ciphertext: aead.Seal(nil, nonce, raw, []byte(label)),
	}
	sealed, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode sealed %s: %w", label, err)
	}
	return writeExclusiveAtomic(path, append(sealed, '\n'))
}

func loadCredentialedJSON(path string, target any, credential string, label string) error {
	if err := requireExact(credential, "credentialed artifact key", 512); err != nil {
		return err
	}
	sealed, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	var envelope credentialedArtifact
	if err := decodeStrictJSON(sealed, &envelope, label+" envelope"); err != nil {
		return err
	}
	if envelope.Schema != credentialedArtifactSchemaV1 {
		return fmt.Errorf("%s credentialed schema is invalid", label)
	}
	aead, err := credentialAEAD(credential)
	if err != nil {
		return err
	}
	if len(envelope.Nonce) != aead.NonceSize() || len(envelope.Ciphertext) == 0 {
		return fmt.Errorf("%s credentialed payload is invalid", label)
	}
	raw, err := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, []byte(label))
	if err != nil {
		return fmt.Errorf("decrypt %s: %w", label, err)
	}
	return decodeStrictJSON(raw, target, label)
}

func credentialAEAD(credential string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(credential))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create credentialed artifact cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
