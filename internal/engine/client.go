// Package engine runs the pinned native Engine with bounded, isolated requests.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Config struct {
	Binary    string `json:"binary"`
	Resources string `json:"resources"`
}

func (c Config) Validate() error {
	if c.Binary != "" && !filepath.IsAbs(c.Binary) {
		return errors.New("engine binary must be an absolute path")
	}
	if c.Resources != "" && !filepath.IsAbs(c.Resources) {
		return errors.New("engine resources must be an absolute path")
	}
	return nil
}

var ErrUnavailable = errors.New("engine_unavailable")
var ErrFailure = errors.New("engine_failure")
var ErrInvalid = errors.New("invalid_engine_request")

// Discard excess output while letting the child finish or reach its deadline.
// exec.Cmd must not accumulate an unbounded native diagnostic in memory.
type boundedOutput struct {
	bytes.Buffer
	exceeded bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (1 << 20) - b.Len()
	if len(p) > remaining {
		b.exceeded = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
func (c Config) Query(ctx context.Context, request any) (json.RawMessage, error) {
	if c.Binary == "" {
		return nil, ErrUnavailable
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > 65536 {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	scratch, err := os.MkdirTemp("", "msime-query-")
	if err != nil {
		return nil, ErrFailure
	}
	defer os.RemoveAll(scratch)
	command := exec.CommandContext(ctx, c.Binary, c.Resources, scratch)
	command.Stdin = bytes.NewReader(raw)
	command.Stderr = io.Discard
	command.WaitDelay = time.Second
	var output boundedOutput
	command.Stdout = &output
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrFailure
	}
	if output.exceeded || !json.Valid(output.Bytes()) {
		return nil, ErrFailure
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(output.Bytes(), &envelope) != nil {
		return nil, ErrFailure
	}
	switch envelope.Error {
	case "":
		return json.RawMessage(output.Bytes()), nil
	case "resources_unavailable":
		return nil, ErrUnavailable
	case "invalid_request", "invalid_dictionary_entry":
		return nil, ErrInvalid
	default:
		return nil, ErrFailure
	}
}
