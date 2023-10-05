package report

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_toTime(t *testing.T) {
	w := toTime(11.0)

	require.Equal(t, time.Duration(11*time.Nanosecond), w)
}

func Test_toMicro(t *testing.T) {
	w := toMicro(2301.20)

	require.Equal(t, int(2), w)
}
