package message

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

// RecipientLength is the fixed byte length of a Recipient serialization.
const RecipientLength = 96

// Recipient mirrors the Rust struct containing three public keys.
type Recipient struct {
	ClientIdentity      [32]byte
	ClientEncryptionKey [32]byte
	Gateway             [32]byte
}

// RecipientFromBytes constructs a Recipient from the raw 96 byte layout.
func RecipientFromBytes(b []byte) (Recipient, error) {
	if len(b) != RecipientLength {
		return Recipient{}, fmt.Errorf("recipient: invalid length %d", len(b))
	}
	var r Recipient
	copy(r.ClientIdentity[:], b[:32])
	copy(r.ClientEncryptionKey[:], b[32:64])
	copy(r.Gateway[:], b[64:])
	return r, nil
}

// Bytes returns the canonical byte representation.
func (r Recipient) Bytes() []byte {
	out := make([]byte, RecipientLength)
	copy(out[:32], r.ClientIdentity[:])
	copy(out[32:64], r.ClientEncryptionKey[:])
	copy(out[64:], r.Gateway[:])
	return out
}

// String renders the recipient as base58 encoded triple matching Rust formatting.
func (r Recipient) String() string {
	var sb strings.Builder
	sb.Grow(120)
	sb.WriteString(base58.Encode(r.ClientIdentity[:]))
	sb.WriteByte('.')
	sb.WriteString(base58.Encode(r.ClientEncryptionKey[:]))
	sb.WriteByte('@')
	sb.WriteString(base58.Encode(r.Gateway[:]))
	return sb.String()
}

// ParseRecipient parses a base58 string (identity.encryption@gateway) into a Recipient.
func ParseRecipient(s string) (Recipient, error) {
	parts := strings.Split(s, "@")
	if len(parts) != 2 {
		return Recipient{}, fmt.Errorf("recipient: expected single '@'")
	}
	clientParts := strings.Split(parts[0], ".")
	if len(clientParts) != 2 {
		return Recipient{}, fmt.Errorf("recipient: expected single '.' in client half")
	}

	ident, err := decodeBase58To32(clientParts[0])
	if err != nil {
		return Recipient{}, fmt.Errorf("recipient: decode identity: %w", err)
	}
	enc, err := decodeBase58To32(clientParts[1])
	if err != nil {
		return Recipient{}, fmt.Errorf("recipient: decode enc key: %w", err)
	}
	gateway, err := decodeBase58To32(parts[1])
	if err != nil {
		return Recipient{}, fmt.Errorf("recipient: decode gateway: %w", err)
	}

	return Recipient{
		ClientIdentity:      ident,
		ClientEncryptionKey: enc,
		Gateway:             gateway,
	}, nil
}

func decodeBase58To32(s string) ([32]byte, error) {
	decoded, err := base58.Decode(s)
	if err != nil {
		return [32]byte{}, err
	}
	if len(decoded) != 32 {
		return [32]byte{}, fmt.Errorf("expected 32 decoded bytes, got %d", len(decoded))
	}
	var out [32]byte
	copy(out[:], decoded)
	return out, nil
}

// MarshalText implements encoding.TextMarshaler for human readable formats.
func (r Recipient) MarshalText() ([]byte, error) {
	return []byte(r.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (r *Recipient) UnmarshalText(text []byte) error {
	parsed, err := ParseRecipient(string(text))
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// MarshalBinary returns the binary representation used in protocol messages.
func (r Recipient) MarshalBinary() ([]byte, error) {
	return r.Bytes(), nil
}

// UnmarshalBinary populates from raw bytes.
func (r *Recipient) UnmarshalBinary(data []byte) error {
	parsed, err := RecipientFromBytes(data)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// EncodeBase64 returns a base64 encoded string of the bytes (handy for debugging).
func (r Recipient) EncodeBase64() string {
	return base64.StdEncoding.EncodeToString(r.Bytes())
}
