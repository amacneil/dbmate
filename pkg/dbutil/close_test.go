package dbutil

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func TestCloseOrWarn(t *testing.T) {
	var out bytes.Buffer
	orig := warnOutput
	warnOutput = &out
	t.Cleanup(func() { warnOutput = orig })

	t.Run("close succeeds", func(t *testing.T) {
		out.Reset()
		closed := false
		CloseOrWarn(closerFunc(func() error { closed = true; return nil }))
		require.True(t, closed)
		require.Empty(t, out.String())
	})

	t.Run("close fails", func(t *testing.T) {
		out.Reset()
		err := errors.New("tls: failed to send closeNotify alert (but connection was closed anyway)")
		require.NotPanics(t, func() {
			CloseOrWarn(closerFunc(func() error { return err }))
		})
		require.Equal(t, "Warning: "+err.Error()+"\n", out.String())
	})
}
