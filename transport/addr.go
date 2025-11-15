package transport

import (
	"sync"

	ma "github.com/multiformats/go-multiaddr"
)

const (
	nymProtocolCode = 999
	nymProtocolName = "nym"
)

var registerOnce sync.Once

func init() {
	ensureProtocolRegistered()
}

func ensureProtocolRegistered() {
	registerOnce.Do(func() {
		proto := ma.Protocol{
			Name:       nymProtocolName,
			Code:       nymProtocolCode,
			VCode:      ma.CodeToVarint(nymProtocolCode),
			Size:       ma.LengthPrefixedVarSize,
			Transcoder: ma.NewTranscoderFromFunctions(stringToBytes, bytesToString, validateBytes),
		}
		if err := ma.AddProtocol(proto); err != nil {
			// If the protocol already exists, ignore, otherwise panic as transport cannot function.
			if err.Error() != "protocol by the name \"nym\" already exists" && err.Error() != "protocol code 999 already taken by \"nym\"" {
				panic(err)
			}
		}
	})
}

func stringToBytes(s string) ([]byte, error) {
	return []byte(s), nil
}

func bytesToString(b []byte) (string, error) {
	return string(b), nil
}

func validateBytes(b []byte) error {
	// No additional validation for now; recipient parsing happens later.
	return nil
}
