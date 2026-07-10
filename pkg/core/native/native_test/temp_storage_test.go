package native_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/compiler"
	"github.com/nspcc-dev/neo-go/pkg/config"
	"github.com/nspcc-dev/neo-go/pkg/core/native/nativehashes"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/stretchr/testify/require"
)

func newTempStorageClient(t *testing.T) *neotest.ContractInvoker {
	return newCustomTempStorageClient(t, func(cfg *config.Blockchain) {
		cfg.Hardforks = map[string]uint32{
			config.HFHuyao.String(): 0,
		}
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
			"github.com/nspcc-dev/neo-go/pkg/interop/contract"
			"github.com/nspcc-dev/neo-go/pkg/interop/iterator"
			"github.com/nspcc-dev/neo-go/pkg/interop/storage"
		)
		var tempStorageHash = interop.Hash160{
			0xbb, 0xc2, 0x15, 0x1b, 0xe9, 0x1a, 0x8b, 0x9c, 0xad, 0x8e, 0xa3, 0x5b, 0xa5, 0xd6, 0x72, 0xfc, 0x01, 0x35, 0x46, 0x93,
		}
		func Put(key, value []byte, validTill int) {
			contract.Call(tempStorageHash, "put", contract.WriteStates, key, value, validTill)
		}
		func Get(key []byte) []byte {
			return contract.Call(tempStorageHash, "get", contract.ReadStates, key).([]byte)
		}
		func GetByHash(hash interop.Hash160, key []byte) []byte {
			return contract.Call(tempStorageHash, "get", contract.ReadStates, hash, key).([]byte)
		}
		func GetExpiration(key []byte) int {
			return contract.Call(tempStorageHash, "getExpiration", contract.ReadStates, key).(int)
		}
		func GetExpirationByHash(hash interop.Hash160, key []byte) int {
			return contract.Call(tempStorageHash, "getExpiration", contract.ReadStates, hash, key).(int)
		}
		func Delete(key []byte) {
			contract.Call(tempStorageHash, "delete", contract.WriteStates, key)
		}
		func FindValues(prefix []byte) [][]byte {
			i := contract.Call(tempStorageHash, "find", contract.ReadStates, prefix, storage.ValuesOnly).(iterator.Iterator)
			var res [][]byte
			for iterator.Next(i) {
				res = append(res, iterator.Value(i).([]byte))
			}
			return res
		}
		func FindValuesByHash(hash interop.Hash160, prefix []byte) [][]byte {
			i := contract.Call(tempStorageHash, "find", contract.ReadStates, hash, prefix, storage.ValuesOnly).(iterator.Iterator)
			var res [][]byte
			for iterator.Next(i) {
				res = append(res, iterator.Value(i).([]byte))
			}
			return res
		}
		func Renew(key []byte, validTill int) {
			contract.Call(tempStorageHash, "renew", contract.WriteStates, key, validTill)
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
	const sysFee = 10000_0000
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
	tempStorageInvoker.InvokeWithFeeFail(t, fmt.Sprintf("System.Contract.Call failed: called contract %s not found: key not found", nativehashes.TemporaryStorage.StringLE()), sysFee, "put", key, value, till)

	// Invoke at Huyao should fail.
	tempStorageInvoker.InvokeWithFeeFail(t, "System.Contract.CallNative failed: native contract TemporaryStorage is active after hardfork Huyao", sysFee, "put", key, value, till)

	// Invoke after Huyao should succeed.
	tempStorageInvoker.InvokeWithFee(t, stackitem.Null{}, sysFee, "put", key, value, till)
}

func TestTempStorage_ManifestMethods(t *testing.T) {
	c := newTempStorageClient(t)
	tempStorageInvoker, ctrHash := getTempStorageInvoker(t, c)
	key1 := []byte("aa1")
	value1 := []byte("one")
	key2 := []byte("aa2")
	value2 := []byte("two")

	topTimestamp := c.TopBlock(t).Timestamp
	msPerBlock := uint64(c.Chain.GetMillisecondsPerBlock())

	validTill1 := topTimestamp + 4*msPerBlock
	validTill2 := topTimestamp + 5*msPerBlock
	validTillRenewed := topTimestamp + 6*msPerBlock
	minValidTill := topTimestamp + 2*msPerBlock
	maxValidTill := topTimestamp + uint64(c.Executor.Chain.GetConfig().Genesis.TemporaryStorageMaxTTL/time.Millisecond)

	tempStorageInvoker.Invoke(t, stackitem.Null{}, "put", key1, value1, validTill1)
	tempStorageInvoker.InvokeAndCheck(t, checkBytes(value1), "get", key1)
	tempStorageInvoker.InvokeAndCheck(t, checkBytes(value1), "getByHash", ctrHash, key1)
	tempStorageInvoker.Invoke(t, int(validTill1), "getExpiration", key1)
	tempStorageInvoker.Invoke(t, int(validTill1), "getExpirationByHash", ctrHash, key1)

	tempStorageInvoker.Invoke(t, stackitem.Null{}, "put", key2, value2, validTill2)
	tempStorageInvoker.InvokeAndCheck(t, checkByteArraySlice([][]byte{value1, value2}), "findValues", []byte("aa"))
	tempStorageInvoker.InvokeAndCheck(t, checkByteArraySlice([][]byte{value1, value2}), "findValuesByHash", ctrHash, []byte("aa"))

	tempStorageInvoker.Invoke(t, stackitem.Null{}, "renew", key1, validTillRenewed)
	tempStorageInvoker.Invoke(t, int(validTillRenewed), "getExpiration", key1)

	tempStorageInvoker.Invoke(t, stackitem.Null{}, "delete", key1)
	tempStorageInvoker.Invoke(t, 0, "getExpiration", key1)

	tempStorageInvoker.InvokeFail(t, "item is valid for less than 2*msPerBlock", "put", []byte("low"), []byte("v"), minValidTill-1)
	tempStorageInvoker.InvokeFail(t, "validTill exceeds max limit", "put", []byte("high"), []byte("v"), maxValidTill+1)
	tempStorageInvoker.InvokeFail(t, "failed to get old record", "renew", []byte("missing"), validTillRenewed)
}

func TestTempStorage_PostPersistCleanup(t *testing.T) {
	c := newTempStorageClient(t)
	tempStorageInvoker, _ := getTempStorageInvoker(t, c)

	key := []byte("ephemeral")
	value := []byte("payload")
	msPerBlock := uint64(c.Chain.GetMillisecondsPerBlock())
	topTimestamp := c.TopBlock(t).Timestamp
	validTill := topTimestamp + 3*msPerBlock
	renewedTill := validTill + msPerBlock

	tempStorageInvoker.Invoke(t, stackitem.Null{}, "put", key, value, validTill)
	tempStorageInvoker.Invoke(t, stackitem.Null{}, "renew", key, renewedTill)

	for c.TopBlock(t).Timestamp <= renewedTill {
		c.Executor.AddNewBlock(t)
	}
	tempStorageInvoker.InvokeFail(t, "failed to get old record", "renew", key, c.TopBlock(t).Timestamp+3*msPerBlock)
}

func checkBytes(expected []byte) func(t testing.TB, stack []stackitem.Item) {
	return func(t testing.TB, stack []stackitem.Item) {
		require.Len(t, stack, 1)
		actual, err := stack[0].TryBytes()
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
}

func checkByteArraySlice(expected [][]byte) func(t testing.TB, stack []stackitem.Item) {
	return func(t testing.TB, stack []stackitem.Item) {
		require.Len(t, stack, 1)
		arr, ok := stack[0].Value().([]stackitem.Item)
		require.True(t, ok)
		require.Len(t, arr, len(expected))
		for i := range arr {
			actual, err := arr[i].TryBytes()
			require.NoError(t, err)
			require.Equal(t, expected[i], actual)
		}
	}
}
