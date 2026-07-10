package native_test

import (
	"math"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/native"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/stretchr/testify/require"
)

func TestTempStorage_CalculateStoragePrice(t *testing.T) {
	const year = 365 * 24 * 60 * 60 * 1000 // milliseconds.
	s := native.NewTempStorage()
	bc, _, _ := chain.NewMulti(t)
	ic, err := bc.GetTestVM(trigger.Application, nil, &block.Block{Header: block.Header{Timestamp: 1}})
	require.NoError(t, err)

	permanent := ic.BaseStorageFee()
	minimal := permanent / 10
	require.Positive(t, minimal)

	minKey := []byte{0x00}
	minValue := []byte{}

	// TTL: 0.
	// Size: 1 byte.
	actual := s.CalculateStoragePrice(ic, minKey, minValue, 0)
	require.Equal(t, minimal, actual)

	// TTL: max.
	// Size: 1 byte.
	actual = s.CalculateStoragePrice(ic, minKey, minValue, math.MaxInt64)
	require.Equal(t, permanent, actual)

	// TTL: 1/2 of year.
	// Size: 1 byte.
	actual = s.CalculateStoragePrice(ic, minKey, minValue, year/2)
	require.Equal(t, permanent/2, actual)

	// TTL: 10% of year.
	// Size: 1 byte.
	actual = s.CalculateStoragePrice(ic, minKey, minValue, year/10)
	require.Equal(t, permanent/10, actual)

	// TTL: <10% of year.
	// Size: 1 byte.
	actual = s.CalculateStoragePrice(ic, minKey, minValue, year/10-1)
	require.Equal(t, permanent/10, actual)
}
