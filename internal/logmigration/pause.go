package logmigration

import (
	"context"
	"io"
	"os"
	"time"
)

// Wait permits the operator or health watchdog to pause between bounded operations.
func Wait(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	for {
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

type Reader struct {
	Context context.Context
	Path    string
	Input   io.Reader
}

func (r Reader) Read(p []byte) (int, error) {
	if err := Wait(r.Context, r.Path); err != nil {
		return 0, err
	}
	return r.Input.Read(p)
}
