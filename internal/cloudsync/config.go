package cloudsync

import (
	"bytes"
	"compress/gzip"

	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"loris-tunnel/internal/conf"
)

const SchemaVersion = 2
const MaxFileSize = 1 << 20
const MaxWireSize = 2 << 20

var ErrBackupContainsCredentials = errors.New("cloud backup blocked: configuration contains an SSH password, private key path, SSH agent socket path, or license code")

// Config carries a compressed TOML file over HTTPS.
type Config struct {
	SchemaVersion int    `json:"schema_version"`
	Data          string `json:"data"`
	OriginalSize  int    `json:"original_size"`
}

func Fingerprint(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func Pack(data []byte) (*Config, error) {
	if len(data) > MaxFileSize {
		return nil, fmt.Errorf("TOML exceeds 1 MiB")
	}
	cfg, e := conf.ParseConfigTOML(data)
	if e != nil {
		return nil, e
	}
	if cfg.License.Code != "" {
		return nil, ErrBackupContainsCredentials
	}
	for _, jumper := range cfg.Jumpers {
		if jumper.Password != "" || jumper.KeyPath != "" || jumper.AgentSocketPath != "" {
			return nil, ErrBackupContainsCredentials
		}
	}
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if _, e := z.Write(data); e != nil {
		return nil, e
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	return &Config{SchemaVersion, base64.StdEncoding.EncodeToString(buf.Bytes()), len(data)}, nil
}
func (c Config) Unpack() ([]byte, error) {
	if c.SchemaVersion != SchemaVersion || c.OriginalSize < 0 || c.OriginalSize > MaxFileSize || len(c.Data) > MaxWireSize {
		return nil, fmt.Errorf("unsupported or oversized file backup; upgrade client and server")
	}
	packed, e := base64.StdEncoding.DecodeString(c.Data)
	if e != nil {
		return nil, fmt.Errorf("invalid backup encoding")
	}

	z, e := gzip.NewReader(bytes.NewReader(packed))
	if e != nil {
		return nil, fmt.Errorf("invalid compressed backup")
	}
	defer z.Close()
	data, e := io.ReadAll(io.LimitReader(z, MaxFileSize+1))
	if e != nil || len(data) > MaxFileSize || len(data) != c.OriginalSize {
		return nil, fmt.Errorf("invalid or oversized decompressed backup")
	}
	if _, e = conf.ParseConfigTOML(data); e != nil {
		return nil, e
	}
	return data, nil
}
