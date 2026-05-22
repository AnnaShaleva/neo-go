package native_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/compiler"
	"github.com/nspcc-dev/neo-go/pkg/config"
	"github.com/nspcc-dev/neo-go/pkg/core/native/nativehashes"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
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

func getTempStorageInvoker(t *testing.T, tempStorageC *neotest.ContractInvoker) *neotest.ContractInvoker {
	src := `package tempstorageinvoker
		import (
			"github.com/nspcc-dev/neo-go/pkg/interop/native/temp_storage"
		)
		func Put(key, value []byte, validTill int) {
			// TODO
		}
		func Get(key []byte) []byte {
			// TODO
		}
		// TODO
`
	e := tempStorageC.Executor
	ctr := neotest.CompileSource(t, e.Validator.ScriptHash(), strings.NewReader(src), &compiler.Options{
		Name: "tempstorageinvoker",
	})
	e.DeployContract(t, ctr, nil)
	ctrInvoker := e.NewInvoker(ctr.Hash, e.Committee)
	return ctrInvoker
}

func TestTempStorage_Activation(t *testing.T) {
	c := newCustomTempStorageClient(t, func(cfg *config.Blockchain) {
		cfg.Hardforks = map[string]uint32{
			config.HFHuyao.String(): 2,
		}
	})
	till := c.TopBlock(t).Timestamp + uint64(10*c.Chain.GetMillisecondsPerBlock())

	tempStorageInvoker := getTempStorageInvoker(t, c)

	// Invoke before Huyao should fail.
	tempStorageInvoker.InvokeFail(t, fmt.Sprintf("called contract %s not found: key not found", nativehashes.TemporaryStorage.StringLE()), "put", 1, 2, till)

	// Invoke at Huyao should fail.
	tempStorageInvoker.InvokeWithFeeFail(t, "System.Contract.CallNative failed: native contract TemporaryStorage is active after hardfork Huyao", 10000_0000, "put", 1, 2, till)

	// Invoke after Huyao should succeed.
	tempStorageInvoker.Invoke(t, true, "put", 1, 2, till)
}
