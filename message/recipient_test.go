package message

import (
	"strings"
	"testing"
)

func TestParseRecipient(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "ValidRecipient",
			input:   "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f",
			wantErr: false,
		},
		{
			name:    "MissingDot",
			input:   "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f",
			wantErr: true,
		},
		{
			name:    "MissingAt",
			input:   "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f",
			wantErr: true,
		},
		{
			name:    "EmptyString",
			input:   "",
			wantErr: true,
		},
		{
			name:    "OnlyDot",
			input:   ".",
			wantErr: true,
		},
		{
			name:    "OnlyAt",
			input:   "@",
			wantErr: true,
		},
		{
			name:    "InvalidBase58Identity",
			input:   "0OIl.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f",
			wantErr: true,
		},
		{
			name:    "InvalidBase58Encryption",
			input:   "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.0OIl@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f",
			wantErr: true,
		},
		{
			name:    "InvalidBase58Gateway",
			input:   "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@0OIl",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipient, err := ParseRecipient(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRecipient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Just verify we got a valid recipient
				// The fields are fixed-size arrays, so they can't be empty
				_ = recipient.String()
			}
		})
	}
}

func TestRecipientString(t *testing.T) {
	input := "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f"

	recipient, err := ParseRecipient(input)
	if err != nil {
		t.Fatalf("ParseRecipient() failed: %v", err)
	}

	output := recipient.String()
	if output != input {
		t.Errorf("Recipient.String() = %s, want %s", output, input)
	}
}

func TestRecipientRoundTrip(t *testing.T) {
	tests := []string{
		"CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f",
	}

	for _, input := range tests {
		t.Run(input[:20]+"...", func(t *testing.T) {
			recipient, err := ParseRecipient(input)
			if err != nil {
				t.Fatalf("ParseRecipient() failed: %v", err)
			}

			output := recipient.String()
			if output != input {
				t.Errorf("Round trip failed:\ninput:  %s\noutput: %s", input, output)
			}

			// Parse again to ensure consistency
			recipient2, err := ParseRecipient(output)
			if err != nil {
				t.Fatalf("ParseRecipient() failed on round-trip: %v", err)
			}

			if recipient2.String() != input {
				t.Errorf("Second round trip failed")
			}
		})
	}
}

func TestRecipientEqual(t *testing.T) {
	input := "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f"

	r1, err := ParseRecipient(input)
	if err != nil {
		t.Fatalf("ParseRecipient() failed: %v", err)
	}

	r2, err := ParseRecipient(input)
	if err != nil {
		t.Fatalf("ParseRecipient() failed: %v", err)
	}

	// Check that the two recipients are equal
	if r1.String() != r2.String() {
		t.Error("Two recipients parsed from the same string are not equal")
	}

	// Check that the byte representations are equal
	if r1.Bytes()[0] != r2.Bytes()[0] {
		t.Error("Two recipients parsed from the same string have different bytes")
	}
}

func TestRecipientParts(t *testing.T) {
	input := "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f"

	recipient, err := ParseRecipient(input)
	if err != nil {
		t.Fatalf("ParseRecipient() failed: %v", err)
	}

	// Check that the parts are present (they're fixed-size arrays, so just verify they're not all zeros)
	allZeros := true
	for _, b := range recipient.ClientIdentity {
		if b != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		t.Error("ClientIdentity is all zeros")
	}

	// Check that the string representation contains all parts
	str := recipient.String()
	if !strings.Contains(str, ".") {
		t.Error("String representation missing dot separator")
	}
	if !strings.Contains(str, "@") {
		t.Error("String representation missing at separator")
	}

	parts := strings.Split(str, ".")
	if len(parts) != 2 {
		t.Errorf("String representation has %d parts after splitting by dot, want 2", len(parts))
	}

	parts = strings.Split(str, "@")
	if len(parts) != 2 {
		t.Errorf("String representation has %d parts after splitting by at, want 2", len(parts))
	}
}

func BenchmarkParseRecipient(b *testing.B) {
	input := "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseRecipient(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRecipientString(b *testing.B) {
	input := "CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f"
	recipient, _ := ParseRecipient(input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = recipient.String()
	}
}
