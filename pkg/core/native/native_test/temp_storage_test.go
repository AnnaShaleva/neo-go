package native_test

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/compiler"
	"github.com/nspcc-dev/neo-go/pkg/config"
	"github.com/nspcc-dev/neo-go/pkg/config/limits"
	istorage "github.com/nspcc-dev/neo-go/pkg/core/interop/storage"
	"github.com/nspcc-dev/neo-go/pkg/core/native"
	"github.com/nspcc-dev/neo-go/pkg/core/native/nativehashes"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/stretchr/testify/require"
)

// maxTempStorageTTL is the maximum allowed TemporaryStorageMaxTTL value.
const maxTempStorageTTL = 365 * 24 * time.Hour

func newTempStorageClient(t *testing.T) *neotest.ContractInvoker {
	return newCustomTempStorageClient(t, func(cfg *config.Blockchain) {
		cfg.Hardforks = map[string]uint32{
			config.HFHuyao.String(): 0,
		}
		cfg.Genesis.TemporaryStorageMaxTTL = maxTempStorageTTL
	})
}

func newCustomTempStorageClient(t *testing.T, f func(cfg *config.Blockchain)) *neotest.ContractInvoker {
	bc, acc := chain.NewSingleWithCustomConfig(t, f)
	e := neotest.NewExecutor(t, bc, acc, acc)

	return e.CommitteeInvoker(nativehashes.TemporaryStorage)
}

func getTempStorageInvoker(t *testing.T, tempStorageC *neotest.ContractInvoker) (*neotest.ContractInvoker, util.Uint160) {
	src := `package tempstorageinvoker
		import (
			"github.com/nspcc-dev/neo-go/pkg/interop"
			"github.com/nspcc-dev/neo-go/pkg/interop/iterator"
			"github.com/nspcc-dev/neo-go/pkg/interop/storage"
			"github.com/nspcc-dev/neo-go/pkg/interop/native/tempstorage"
		)
		func Put(key, value []byte, validTill int) {
			tempstorage.Put(key, value, validTill)
		}
		func Get(key []byte) []byte {
			return tempstorage.Get(key)
		}
		func GetByHash(hash interop.Hash160, key []byte) []byte {
			return tempstorage.GetByHash(hash, key)
		}
		func GetExpiration(key []byte) int {
			return tempstorage.GetExpiration(key)
		}
		func GetExpirationByHash(hash interop.Hash160, key []byte) int {
			return tempstorage.GetExpirationByHash(hash, key)
		}
		func Delete(key []byte) {
			tempstorage.Delete(key)
		}
		func FindValues(prefix []byte, opts storage.FindFlags) [][]byte {
			i := tempstorage.Find(prefix, opts)
			var res [][]byte
			for iterator.Next(i) {
				res = append(res, iterator.Value(i).([]byte))
			}
			return res
		}
		func FindValuesByHash(hash interop.Hash160, prefix []byte, opts storage.FindFlags) [][]byte {
			i := tempstorage.FindByHash(hash, prefix, opts)
			var res [][]byte
			for iterator.Next(i) {
				res = append(res, iterator.Value(i).([]byte))
			}
			return res
		}
		func Renew(key []byte, validTill int) {
			tempstorage.Renew(key, validTill)
		}
	`
	e := tempStorageC.Executor
	ctr := neotest.CompileSource(t, e.Validator.ScriptHash(), strings.NewReader(src), &compiler.Options{
		Name: "tempstorageinvoker",
		Permissions: []manifest.Permission{
			*manifest.NewPermission(manifest.PermissionWildcard),
		},
	})
	e.DeployContract(t, ctr, nil)
	ctrInvoker := e.NewInvoker(ctr.Hash, e.Committee)
	return ctrInvoker, ctr.Hash
}

func TestTempStorage_Activation(t *testing.T) {
	c := newCustomTempStorageClient(t, func(cfg *config.Blockchain) {
		cfg.Hardforks = map[string]uint32{
			config.HFHuyao.String(): 3,
		}
	})
	till := c.TopBlock(t).Timestamp + uint64(10*c.Chain.GetMillisecondsPerBlock())

	tempStorageInvoker, _ := getTempStorageInvoker(t, c)
	key := []byte{1}
	value := []byte{2}

	// Invoke before Huyao should fail.
	tempStorageInvoker.InvokeWithFeeFail(t, fmt.Sprintf("token contract %s not found: key not found", nativehashes.TemporaryStorage.StringLE()), 10000_0000, "put", key, value, till)

	// Invoke at Huyao should fail.
	tempStorageInvoker.InvokeWithFeeFail(t, "System.Contract.CallNative failed: native contract TemporaryStorage is active after hardfork Huyao", 10000_0000, "put", key, value, till)

	// Invoke after Huyao should succeed.
	tempStorageInvoker.Invoke(t, stackitem.Null{}, "put", key, value, till)
}

func TestTempStorage_PutGetRenewDelete(t *testing.T) {
	c := newTempStorageClient(t)
	tmp, ctrHash := getTempStorageInvoker(t, c)
	key1 := []byte("aa1")
	value1 := []byte("one")
	key2 := []byte("aa2")
	value2 := []byte("two")
	key3 := []byte("bb")
	value3 := []byte("three")
	key4 := []byte("cc") // available for a single block only.
	value4 := []byte("four")

	topTimestamp := c.TopBlock(t).Timestamp
	msPerBlock := uint64(c.Chain.GetMillisecondsPerBlock())

	// put: good.
	validTill1 := topTimestamp + 4*msPerBlock
	validTill2 := topTimestamp + 5*msPerBlock
	validTillRenewed := topTimestamp + 6*msPerBlock
	tmp.Invoke(t, stackitem.Null{}, "put", key1, value1, validTill1)
	tmp.Invoke(t, stackitem.Null{}, "put", key2, value2, validTill2)
	tmp.Invoke(t, stackitem.Null{}, "put", key3, value3, validTill2)

	// get*: existing entry.
	tmp.Invoke(t, stackitem.Make(value1), "get", key1)
	tmp.Invoke(t, stackitem.Make(value1), "getByHash", ctrHash, key1)

	// getExpiration*: existing entry.
	tmp.Invoke(t, int(validTill1), "getExpiration", key1)
	tmp.Invoke(t, int(validTill1), "getExpirationByHash", ctrHash, key1)

	// get: retrieve the item available at the current block only.
	tx1 := tmp.PrepareInvoke(t, "put", key4, value4, c.TopBlock(t).Timestamp+1)
	tx2 := tmp.PrepareInvoke(t, "get", key4)
	tmp.AddNewBlock(t, tx1, tx2)
	c.CheckHalt(t, tx2.Hash(), stackitem.Make(value4))
	tmp.Invoke(t, stackitem.Null{}, "get", key4)

	// find: remove prefix.
	tmp.Invoke(t, stackitem.Make([]any{stackitem.NewBuffer(value1), stackitem.NewBuffer(value2)}), "findValues", []byte("aa"), istorage.FindValuesOnly)
	tmp.Invoke(t, stackitem.Make([]any{stackitem.NewBuffer(value1), stackitem.NewBuffer(value2)}), "findValuesByHash", ctrHash, []byte("aa"), istorage.FindValuesOnly)

	// renew: existing entry.
	tmp.Invoke(t, stackitem.Null{}, "renew", key1, validTillRenewed)
	tmp.Invoke(t, int(validTillRenewed), "getExpiration", key1)

	// delete: existing entry, getExpiration of missing entry.
	tmp.Invoke(t, stackitem.Null{}, "delete", key1)
	tmp.Invoke(t, 0, "getExpiration", key1)

	// Error cases.
	t.Run("put", func(t *testing.T) {
		maxValidTill := c.TopBlock(t).Timestamp + uint64(c.Chain.GetConfig().Genesis.TemporaryStorageMaxTTL.Milliseconds())
		t.Run("large key", func(t *testing.T) {
			tmp.InvokeFail(t, "input is too big: 65 vs 64", "put", make([]byte, limits.MaxStorageKeyLen+1), []byte("v"), maxValidTill)
		})
		t.Run("large validTill", func(t *testing.T) {
			tmp.InvokeFail(t, "validTill exceeds max limit", "put", []byte("high"), []byte("v"), maxValidTill+1000)
		})
		t.Run("not enough gas", func(t *testing.T) {
			tmp.InvokeWithFeeFail(t, "failed to charge temporary storage fee", 1_5000000, "put", []byte("nogas"), make([]byte, 1000), maxValidTill)
		})
		t.Run("caller is not a contract", func(t *testing.T) {
			c.InvokeFail(t, "failed to get calling contract", "put", []byte("k"), []byte("v"), maxValidTill)
		})
	})
	t.Run("renew", func(t *testing.T) {
		t.Run("unexisting entry", func(t *testing.T) {
			tmp.InvokeFail(t, "failed to get old record", "renew", []byte("missing"), validTillRenewed)
		})
		t.Run("large key", func(t *testing.T) {
			tmp.InvokeFail(t, "input is too big", "renew", make([]byte, limits.MaxStorageKeyLen+1), validTillRenewed)
		})
		t.Run("large validTill", func(t *testing.T) {
			maxValidTill := c.TopBlock(t).Timestamp + uint64(c.Chain.GetConfig().Genesis.TemporaryStorageMaxTTL.Milliseconds())
			tmp.InvokeFail(t, "validTill exceeds max limit", "renew", key2, maxValidTill+1000)
		})
		t.Run("not newer", func(t *testing.T) {
			tmp.InvokeFail(t, "new expiration point should be newer than the old one", "renew", key2, validTill2)
			tmp.InvokeFail(t, "new expiration point should be newer than the old one", "renew", key2, validTill2-1)
		})
		t.Run("expired entry", func(t *testing.T) {
			tmp.Invoke(t, stackitem.Null{}, "put", []byte("short"), []byte("v"), c.TopBlock(t).Timestamp+1)
			tmp.InvokeFail(t, "already expired", "renew", []byte("short"), validTillRenewed)
		})
		t.Run("caller is not a contract", func(t *testing.T) {
			c.InvokeFail(t, "failed to get calling contract", "renew", key2, validTillRenewed)
		})
	})
	t.Run("delete", func(t *testing.T) {
		t.Run("large key", func(t *testing.T) {
			tmp.InvokeFail(t, "input is too big", "delete", make([]byte, limits.MaxStorageKeyLen+1))
		})
		t.Run("missing entry", func(t *testing.T) {
			tmp.Invoke(t, stackitem.Null{}, "delete", []byte("missing"))
		})
		t.Run("removes the entry and its expiration", func(t *testing.T) {
			tmp.Invoke(t, stackitem.Null{}, "put", []byte("del"), []byte("v"), validTill2)
			tmp.Invoke(t, stackitem.Null{}, "delete", []byte("del"))
			tmp.Invoke(t, stackitem.Null{}, "get", []byte("del"))
			tmp.Invoke(t, 0, "getExpiration", []byte("del"))
		})
		t.Run("caller is not a contract", func(t *testing.T) {
			c.InvokeFail(t, "failed to get calling contract", "delete", key2)
		})
	})
}

func TestTempStorage_PostPersistCleanup(t *testing.T) {
	c := newTempStorageClient(t)
	tmp, _ := getTempStorageInvoker(t, c)

	key1 := []byte("a1")
	value1 := []byte("1")
	key2 := []byte("a2")
	value2 := []byte("2")
	msPerBlock := uint64(c.Chain.GetMillisecondsPerBlock())
	validTill := c.TopBlock(t).Timestamp + 5*msPerBlock
	renewedTill := validTill + msPerBlock

	tmp.Invoke(t, stackitem.Null{}, "put", key1, value1, validTill)
	tmp.Invoke(t, stackitem.Null{}, "put", key2, value2, renewedTill+msPerBlock)
	tmp.Invoke(t, stackitem.Make([]any{stackitem.NewBuffer(value1), stackitem.NewBuffer(value2)}), "findValues", []byte("a"), istorage.FindValuesOnly)
	tmp.Invoke(t, stackitem.Null{}, "renew", key1, renewedTill)

	b := c.NewUnsignedBlock(t)
	b.Timestamp = renewedTill + 1
	c.SignBlock(b)
	require.NoError(t, c.Chain.AddBlock(b))

	tmp.Invoke(t, stackitem.Make([]any{stackitem.NewBuffer(value2)}), "findValues", []byte("a"), istorage.FindValuesOnly)
	tmp.InvokeFail(t, "failed to get old record", "renew", key1, c.TopBlock(t).Timestamp+3*msPerBlock)
}

func TestTempStorage_RenewedItemSurvivesCleanup(t *testing.T) {
	c := newTempStorageClient(t)
	tmp, _ := getTempStorageInvoker(t, c)

	key := []byte("a")
	value := []byte("1")
	msPerBlock := uint64(c.Chain.GetMillisecondsPerBlock())
	oldTill := c.TopBlock(t).Timestamp + 5*msPerBlock
	newTill := oldTill + 10*msPerBlock

	tmp.Invoke(t, stackitem.Null{}, "put", key, value, oldTill)
	tmp.Invoke(t, stackitem.Null{}, "renew", key, newTill)

	// Persist block that is newer than the old expiration point, but older than the new one.
	b := c.NewUnsignedBlock(t)
	b.Timestamp = oldTill + 1
	c.SignBlock(b)
	require.NoError(t, c.Chain.AddBlock(b))

	tmp.Invoke(t, stackitem.Make(value), "get", key)
	tmp.Invoke(t, int(newTill), "getExpiration", key)

	// Overwriting the item with a lower expiration must not leave stale expiration records either.
	lowerTill := c.TopBlock(t).Timestamp + 3*msPerBlock
	tmp.Invoke(t, stackitem.Null{}, "put", key, value, lowerTill)
	b = c.NewUnsignedBlock(t)
	b.Timestamp = newTill + 1
	c.SignBlock(b)
	require.NoError(t, c.Chain.AddBlock(b))
	tmp.Invoke(t, stackitem.Null{}, "get", key)
}

// tempStoragePicoPerDatoshi is the number of picoGAS units in one datoshi.
const tempStoragePicoPerDatoshi = 10000

// recordKeyPrefixLen is the length of the record storage key prefix (record
// prefix byte and contract ID) that is charged together with the user key.
const recordKeyPrefixLen = 5

// expectedTempStoragePrice returns the exact price (in datoshi, rounded up) of
// storing the specified number of chargeable bytes for the specified lifetime
// (in milliseconds) given the default permanent storage price.
func expectedTempStoragePrice(chargeableBytes int64, lifetime uint64) int64 {
	maxTTL := big.NewInt(maxTempStorageTTL.Milliseconds())
	permanent := new(big.Int).Mul(big.NewInt(chargeableBytes), big.NewInt(native.DefaultStoragePrice*tempStoragePicoPerDatoshi))
	actual := new(big.Int).Mul(permanent, big.NewInt(int64(min(lifetime, uint64(maxTTL.Int64())))))
	actual.Div(actual, maxTTL)
	floor := new(big.Int).Div(permanent, big.NewInt(10))
	if actual.Cmp(floor) < 0 {
		actual = floor
	}
	datoshi := new(big.Int).Add(actual, big.NewInt(tempStoragePicoPerDatoshi-1))
	return datoshi.Div(datoshi, big.NewInt(tempStoragePicoPerDatoshi)).Int64()
}

// expectedOverwriteChargeableBytes returns the number of bytes charged for
// overwriting existing value of size oldLen with the value of size newLen.
func expectedOverwriteChargeableBytes(oldLen, newLen int64) int64 {
	switch {
	case newLen == 0:
		return 0
	case newLen <= oldLen:
		return (newLen-1)/4 + 1
	case oldLen != 0:
		return (oldLen-1)/4 + 1 + newLen - oldLen
	default:
		return newLen
	}
}

func tempStorageTxGas(t *testing.T, tmp *neotest.ContractInvoker, method string, args ...any) int64 {
	h := tmp.Invoke(t, stackitem.Null{}, method, args...)
	return tmp.GetTxExecResult(t, h).GasConsumed
}

func TestTempStorage_Price(t *testing.T) {
	const (
		maxMs = uint64(maxTempStorageTTL / time.Millisecond)
		day   = uint64(24 * time.Hour / time.Millisecond)
		// largeValueLen is the large value size that fits into the transaction script.
		largeValueLen = 60000
	)
	c := newTempStorageClient(t)
	tmp, _ := getTempStorageInvoker(t, c)

	// nextTimestamp is the timestamp of the block that will contain the next transaction.
	nextTimestamp := func() uint64 { return c.TopBlock(t).Timestamp + 1 }
	// makeKey returns a key of the given length that was not used before.
	var keyCounter byte
	makeKey := func(l int) []byte {
		keyCounter++
		k := bytes.Repeat([]byte{'k'}, l)
		k[l-1] = keyCounter
		return k
	}

	// putOverhead returns the gas consumed by `put` call besides the storage price. Fresh
	// key of the same length is stored for the full TTL, which costs exactly the permanent price.
	putOverhead := func(t *testing.T, keyLen, valueLen int) int64 {
		gas := tempStorageTxGas(t, tmp, "put", makeKey(keyLen), make([]byte, valueLen), nextTimestamp()+maxMs)
		return gas - expectedTempStoragePrice(recordKeyPrefixLen+int64(keyLen+valueLen), maxMs)
	}

	t.Run("put new item", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			keyLen   int
			valueLen int
			lifetime uint64
		}{
			{"1+1, max TTL", 1, 1, maxMs},
			{"1+1, half TTL", 1, 1, maxMs / 2},
			{"1+1, floor boundary", 1, 1, maxMs / 10},
			{"1+1, just above floor boundary", 1, 1, maxMs/10 + 1000000},
			{"1+1, below floor", 1, 1, maxMs/10 - 1},
			{"1+1, minimal TTL", 1, 1, 1},
			{"1+1, zero TTL", 1, 1, 0},
			{"10+100, one day", 10, 100, day},
			{"10+100, 100 days", 10, 100, 100 * day},
			{"10+100, 300 days", 10, 100, 300 * day},
			{"64+0, max TTL", limits.MaxStorageKeyLen, 0, maxMs},
			{"64+0, floor", limits.MaxStorageKeyLen, 0, 10},
			{"1+1024, 7 days", 1, 1024, 7 * day},
			{"64+1024, max TTL", limits.MaxStorageKeyLen, 1024, maxMs},
			{"64+1024, floor", limits.MaxStorageKeyLen, 1024, 1000},
			{"7+65535, 123456789 ms", 7, largeValueLen, 123456789},
			{"7+65535, max TTL", 7, largeValueLen, maxMs},
		} {
			t.Run(tc.name, func(t *testing.T) {
				overhead := putOverhead(t, tc.keyLen, tc.valueLen)
				require.Positive(t, overhead)
				key := makeKey(tc.keyLen)
				gas := tempStorageTxGas(t, tmp, "put", key, make([]byte, tc.valueLen), nextTimestamp()+tc.lifetime)
				require.Equal(t, overhead+expectedTempStoragePrice(recordKeyPrefixLen+int64(tc.keyLen+tc.valueLen), tc.lifetime), gas)
			})
		}
	})

	t.Run("put with validTill in the past", func(t *testing.T) {
		overhead := putOverhead(t, 5, 40)
		gas := tempStorageTxGas(t, tmp, "put", makeKey(5), make([]byte, 40), nextTimestamp()-1)
		require.Equal(t, overhead+expectedTempStoragePrice(recordKeyPrefixLen+45, 1), gas)
	})

	t.Run("permanent price relation", func(t *testing.T) {
		// Full TTL costs the same as permanent storage, and the shortest one costs 10% of it.
		require.Equal(t, int64(50*native.DefaultStoragePrice), expectedTempStoragePrice(50, maxMs))
		require.Equal(t, int64(5*native.DefaultStoragePrice), expectedTempStoragePrice(50, 1))
		require.Equal(t, int64(25*native.DefaultStoragePrice), expectedTempStoragePrice(50, maxMs/2))
	})

	t.Run("put overwrite", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			oldLen   int
			newLen   int
			expired  bool
			lifetime uint64
		}{
			{"same size", 100, 100, false, maxMs},
			{"smaller", 100, 10, false, maxMs},
			{"smaller, short", 100, 10, false, day},
			{"larger", 10, 100, false, maxMs},
			{"larger, short", 10, 100, false, 30 * day},
			{"larger, floor", 10, 100, false, 1},
			{"from empty", 0, 20, false, maxMs},
			{"to empty", 20, 0, false, maxMs},
			{"smaller, 1 byte", 4, 1, false, maxMs},
			{"expired, same size", 100, 100, true, maxMs},
			{"expired, smaller", 100, 10, true, day},
			{"expired, larger", 10, 100, true, day},
		} {
			t.Run(tc.name, func(t *testing.T) {
				const keyLen = 8
				overhead := putOverhead(t, keyLen, tc.newLen)
				key := makeKey(keyLen)
				if tc.expired {
					// Reachable for the current block only.
					tmp.Invoke(t, stackitem.Null{}, "put", key, make([]byte, tc.oldLen), nextTimestamp())
				} else {
					tmp.Invoke(t, stackitem.Null{}, "put", key, make([]byte, tc.oldLen), nextTimestamp()+maxMs)
				}
				var chargeable int64
				if tc.expired {
					chargeable = recordKeyPrefixLen + int64(keyLen+tc.newLen)
				} else {
					chargeable = expectedOverwriteChargeableBytes(int64(tc.oldLen), int64(tc.newLen))
				}
				gas := tempStorageTxGas(t, tmp, "put", key, make([]byte, tc.newLen), nextTimestamp()+tc.lifetime)
				require.Equal(t, overhead+expectedTempStoragePrice(chargeable, tc.lifetime), gas)
			})
		}
	})

	t.Run("renew", func(t *testing.T) {
		// renewOverhead returns the gas consumed by `renew` call besides the storage price.
		renewOverhead := func(t *testing.T, keyLen, valueLen int) int64 {
			key := makeKey(keyLen)
			// The item is reachable at the renew block, so the lifetime is exactly maxMs.
			tmp.Invoke(t, stackitem.Null{}, "put", key, make([]byte, valueLen), nextTimestamp()+1)
			gas := tempStorageTxGas(t, tmp, "renew", key, nextTimestamp()+maxMs)
			return gas - expectedTempStoragePrice(expectedOverwriteChargeableBytes(int64(valueLen), int64(valueLen)), maxMs)
		}
		for _, tc := range []struct {
			name     string
			keyLen   int
			valueLen int
			oldLife  uint64 // remaining lifetime of the item at the renew block.
			delta    uint64 // extension of the lifetime.
		}{
			{"max extension", 5, 100, 1, maxMs},
			{"half extension", 5, 100, 1, maxMs / 2},
			{"floor boundary", 5, 100, 1, maxMs / 10},
			{"below floor", 5, 100, 1, maxMs/10 - 1},
			{"minimal extension", 5, 100, 1, 1},
			{"one day", 5, 100, 1, day},
			{"old item with remaining lifetime", 5, 100, 100 * day, 100 * day},
			{"old item, large extension", 5, 100, 100 * day, 265 * day},
			{"empty value is free", 5, 0, 1, maxMs},
			{"1-byte value", 5, 1, 1, maxMs},
			{"4-byte value", 5, 4, 1, maxMs},
			{"5-byte value", 5, 5, 1, maxMs},
			{"max key, 1024-byte value", limits.MaxStorageKeyLen, 1024, 1, 40 * day},
			{"max value", 3, largeValueLen, 1, maxMs},
			{"max value, half", 3, largeValueLen, 1, maxMs / 2},
		} {
			t.Run(tc.name, func(t *testing.T) {
				overhead := renewOverhead(t, tc.keyLen, tc.valueLen)
				require.Positive(t, overhead)
				key := makeKey(tc.keyLen)
				oldTill := nextTimestamp() + tc.oldLife
				tmp.Invoke(t, stackitem.Null{}, "put", key, make([]byte, tc.valueLen), oldTill)
				gas := tempStorageTxGas(t, tmp, "renew", key, oldTill+tc.delta)
				chargeable := expectedOverwriteChargeableBytes(int64(tc.valueLen), int64(tc.valueLen))
				require.Equal(t, overhead+expectedTempStoragePrice(chargeable, tc.delta), gas)
			})
		}
	})
}
