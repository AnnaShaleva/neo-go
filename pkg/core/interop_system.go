package core

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/blockchainer"
	"github.com/nspcc-dev/neo-go/pkg/core/dao"
	"github.com/nspcc-dev/neo-go/pkg/core/interop"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"go.uber.org/zap"
)

const (
	// MaxStorageKeyLen is the maximum length of a key for storage items.
	MaxStorageKeyLen = 1024
	// MaxTraceableBlocks is the maximum number of blocks before current chain
	// height we're able to give information about.
	MaxTraceableBlocks = transaction.MaxValidUntilBlockIncrement
)

// StorageContext contains storing id and read/write flag, it's used as
// a context for storage manipulation functions.
type StorageContext struct {
	ID       int32
	ReadOnly bool
}

// getBlockHashFromElement converts given vm.Element to block hash using given
// Blockchainer if needed. Interop functions accept both block numbers and
// block hashes as parameters, thus this function is needed.
func getBlockHashFromElement(bc blockchainer.Blockchainer, element *vm.Element) (util.Uint256, error) {
	var hash util.Uint256
	hashbytes := element.Bytes()
	if len(hashbytes) <= 5 {
		hashint := element.BigInt().Int64()
		if hashint < 0 || hashint > math.MaxUint32 {
			return hash, errors.New("bad block index")
		}
		hash = bc.GetHeaderHash(int(hashint))
	} else {
		return util.Uint256DecodeBytesBE(hashbytes)
	}
	return hash, nil
}

// blockToStackItem converts block.Block to stackitem.Item
func blockToStackItem(b *block.Block) stackitem.Item {
	return stackitem.NewArray([]stackitem.Item{
		stackitem.NewByteArray(b.Hash().BytesBE()),
		stackitem.NewBigInteger(big.NewInt(int64(b.Version))),
		stackitem.NewByteArray(b.PrevHash.BytesBE()),
		stackitem.NewByteArray(b.MerkleRoot.BytesBE()),
		stackitem.NewBigInteger(big.NewInt(int64(b.Timestamp))),
		stackitem.NewBigInteger(big.NewInt(int64(b.Index))),
		stackitem.NewByteArray(b.NextConsensus.BytesBE()),
		stackitem.NewBigInteger(big.NewInt(int64(len(b.Transactions)))),
	})
}

// bcGetBlock returns current block.
func bcGetBlock(ic *interop.Context, v *vm.VM) error {
	hash, err := getBlockHashFromElement(ic.Chain, v.Estack().Pop())
	if err != nil {
		return err
	}
	block, err := ic.Chain.GetBlock(hash)
	if err != nil || !isTraceableBlock(ic, block.Index) {
		v.Estack().PushVal(stackitem.Null{})
	} else {
		v.Estack().PushVal(blockToStackItem(block))
	}
	return nil
}

// bcGetContract returns contract.
func bcGetContract(ic *interop.Context, v *vm.VM) error {
	hashbytes := v.Estack().Pop().Bytes()
	hash, err := util.Uint160DecodeBytesBE(hashbytes)
	if err != nil {
		return err
	}
	cs, err := ic.DAO.GetContractState(hash)
	if err != nil {
		v.Estack().PushVal([]byte{})
	} else {
		v.Estack().PushVal(stackitem.NewInterop(cs))
	}
	return nil
}

// bcGetHeight returns blockchain height.
func bcGetHeight(ic *interop.Context, v *vm.VM) error {
	v.Estack().PushVal(ic.Chain.BlockHeight())
	return nil
}

// getTransactionAndHeight gets parameter from the vm evaluation stack and
// returns transaction and its height if it's present in the blockchain.
func getTransactionAndHeight(cd *dao.Cached, v *vm.VM) (*transaction.Transaction, uint32, error) {
	hashbytes := v.Estack().Pop().Bytes()
	hash, err := util.Uint256DecodeBytesBE(hashbytes)
	if err != nil {
		return nil, 0, err
	}
	return cd.GetTransaction(hash)
}

// isTraceableBlock defines whether we're able to give information about
// the block with index specified.
func isTraceableBlock(ic *interop.Context, index uint32) bool {
	height := ic.Chain.BlockHeight()
	return index <= height && index+MaxTraceableBlocks > height
}

// transactionToStackItem converts transaction.Transaction to stackitem.Item
func transactionToStackItem(t *transaction.Transaction) stackitem.Item {
	return stackitem.NewArray([]stackitem.Item{
		stackitem.NewByteArray(t.Hash().BytesBE()),
		stackitem.NewBigInteger(big.NewInt(int64(t.Version))),
		stackitem.NewBigInteger(big.NewInt(int64(t.Nonce))),
		stackitem.NewByteArray(t.Sender.BytesBE()),
		stackitem.NewBigInteger(big.NewInt(int64(t.SystemFee))),
		stackitem.NewBigInteger(big.NewInt(int64(t.NetworkFee))),
		stackitem.NewBigInteger(big.NewInt(int64(t.ValidUntilBlock))),
		stackitem.NewByteArray(t.Script),
	})
}

// bcGetTransaction returns transaction.
func bcGetTransaction(ic *interop.Context, v *vm.VM) error {
	tx, h, err := getTransactionAndHeight(ic.DAO, v)
	if err != nil || !isTraceableBlock(ic, h) {
		v.Estack().PushVal(stackitem.Null{})
		return nil
	}
	v.Estack().PushVal(transactionToStackItem(tx))
	return nil
}

// bcGetTransactionFromBlock returns transaction with the given index from the
// block with height or hash specified.
func bcGetTransactionFromBlock(ic *interop.Context, v *vm.VM) error {
	hash, err := getBlockHashFromElement(ic.Chain, v.Estack().Pop())
	if err != nil {
		return err
	}
	block, err := ic.DAO.GetBlock(hash)
	if err != nil || !isTraceableBlock(ic, block.Index) {
		v.Estack().PushVal(stackitem.Null{})
		return nil
	}
	index := v.Estack().Pop().BigInt().Int64()
	if index < 0 || index >= int64(len(block.Transactions)) {
		return errors.New("wrong transaction index")
	}
	tx := block.Transactions[index]
	v.Estack().PushVal(tx.Hash().BytesBE())
	return nil
}

// bcGetTransactionHeight returns transaction height.
func bcGetTransactionHeight(ic *interop.Context, v *vm.VM) error {
	_, h, err := getTransactionAndHeight(ic.DAO, v)
	if err != nil || !isTraceableBlock(ic, h) {
		v.Estack().PushVal(-1)
		return nil
	}
	v.Estack().PushVal(h)
	return nil
}

// engineGetScriptContainer returns transaction that contains the script being
// run.
func engineGetScriptContainer(ic *interop.Context, v *vm.VM) error {
	v.Estack().PushVal(stackitem.NewInterop(ic.Container))
	return nil
}

// engineGetExecutingScriptHash returns executing script hash.
func engineGetExecutingScriptHash(ic *interop.Context, v *vm.VM) error {
	return v.PushContextScriptHash(0)
}

// engineGetCallingScriptHash returns calling script hash.
func engineGetCallingScriptHash(ic *interop.Context, v *vm.VM) error {
	return v.PushContextScriptHash(1)
}

// engineGetEntryScriptHash returns entry script hash.
func engineGetEntryScriptHash(ic *interop.Context, v *vm.VM) error {
	return v.PushContextScriptHash(v.Istack().Len() - 1)
}

// runtimePlatform returns the name of the platform.
func runtimePlatform(ic *interop.Context, v *vm.VM) error {
	v.Estack().PushVal([]byte("NEO"))
	return nil
}

// runtimeGetTrigger returns the script trigger.
func runtimeGetTrigger(ic *interop.Context, v *vm.VM) error {
	v.Estack().PushVal(byte(ic.Trigger))
	return nil
}

// runtimeNotify should pass stack item to the notify plugin to handle it, but
// in neo-go the only meaningful thing to do here is to log.
func runtimeNotify(ic *interop.Context, v *vm.VM) error {
	// It can be just about anything.
	e := v.Estack().Pop()
	item := e.Item()
	// But it has to be serializable, otherwise we either have some broken
	// (recursive) structure inside or an interop item that can't be used
	// outside of the interop subsystem anyway. I'd probably fail transactions
	// that emit such broken notifications, but that might break compatibility
	// with testnet/mainnet, so we're replacing these with error messages.
	_, err := stackitem.SerializeItem(item)
	if err != nil {
		item = stackitem.NewByteArray([]byte(fmt.Sprintf("bad notification: %v", err)))
	}
	ne := state.NotificationEvent{ScriptHash: v.GetCurrentScriptHash(), Item: item}
	ic.Notifications = append(ic.Notifications, ne)
	return nil
}

// runtimeLog logs the message passed.
func runtimeLog(ic *interop.Context, v *vm.VM) error {
	msg := fmt.Sprintf("%q", v.Estack().Pop().Bytes())
	ic.Log.Info("runtime log",
		zap.Stringer("script", v.GetCurrentScriptHash()),
		zap.String("logs", msg))
	return nil
}

// runtimeGetTime returns timestamp of the block being verified, or the latest
// one in the blockchain if no block is given to Context.
func runtimeGetTime(ic *interop.Context, v *vm.VM) error {
	var header *block.Header
	if ic.Block == nil {
		var err error
		header, err = ic.Chain.GetHeader(ic.Chain.CurrentBlockHash())
		if err != nil {
			return err
		}
	} else {
		header = ic.Block.Header()
	}
	v.Estack().PushVal(header.Timestamp)
	return nil
}

// storageDelete deletes stored key-value pair.
func storageDelete(ic *interop.Context, v *vm.VM) error {
	stcInterface := v.Estack().Pop().Value()
	stc, ok := stcInterface.(*StorageContext)
	if !ok {
		return fmt.Errorf("%T is not a StorageContext", stcInterface)
	}
	if stc.ReadOnly {
		return errors.New("StorageContext is read only")
	}
	key := v.Estack().Pop().Bytes()
	si := ic.DAO.GetStorageItem(stc.ID, key)
	if si != nil && si.IsConst {
		return errors.New("storage item is constant")
	}
	return ic.DAO.DeleteStorageItem(stc.ID, key)
}

// storageGet returns stored key-value pair.
func storageGet(ic *interop.Context, v *vm.VM) error {
	stcInterface := v.Estack().Pop().Value()
	stc, ok := stcInterface.(*StorageContext)
	if !ok {
		return fmt.Errorf("%T is not a StorageContext", stcInterface)
	}
	key := v.Estack().Pop().Bytes()
	si := ic.DAO.GetStorageItem(stc.ID, key)
	if si != nil && si.Value != nil {
		v.Estack().PushVal(si.Value)
	} else {
		v.Estack().PushVal(stackitem.Null{})
	}
	return nil
}

// storageGetContext returns storage context (scripthash).
func storageGetContext(ic *interop.Context, v *vm.VM) error {
	contract, err := ic.DAO.GetContractState(v.GetCurrentScriptHash())
	if err != nil {
		return err
	}
	if !contract.HasStorage() {
		return errors.New("contract is not allowed to use storage")
	}
	sc := &StorageContext{
		ID:       contract.ID,
		ReadOnly: false,
	}
	v.Estack().PushVal(stackitem.NewInterop(sc))
	return nil
}

// storageGetReadOnlyContext returns read-only context (scripthash).
func storageGetReadOnlyContext(ic *interop.Context, v *vm.VM) error {
	contract, err := ic.DAO.GetContractState(v.GetCurrentScriptHash())
	if err != nil {
		return err
	}
	if !contract.HasStorage() {
		return err
	}
	sc := &StorageContext{
		ID:       contract.ID,
		ReadOnly: true,
	}
	v.Estack().PushVal(stackitem.NewInterop(sc))
	return nil
}

func putWithContextAndFlags(ic *interop.Context, v *vm.VM, stc *StorageContext, key []byte, value []byte, isConst bool) error {
	if len(key) > MaxStorageKeyLen {
		return errors.New("key is too big")
	}
	if stc.ReadOnly {
		return errors.New("StorageContext is read only")
	}
	si := ic.DAO.GetStorageItem(stc.ID, key)
	if si == nil {
		si = &state.StorageItem{}
	}
	if si.IsConst {
		return errors.New("storage item exists and is read-only")
	}
	sizeInc := 1
	if len(value) > len(si.Value) {
		sizeInc = len(value) - len(si.Value)
	}
	if !v.AddGas(int64(sizeInc) * StoragePrice) {
		return errGasLimitExceeded
	}
	si.Value = value
	si.IsConst = isConst
	return ic.DAO.PutStorageItem(stc.ID, key, si)
}

// storagePutInternal is a unified implementation of storagePut and storagePutEx.
func storagePutInternal(ic *interop.Context, v *vm.VM, getFlag bool) error {
	stcInterface := v.Estack().Pop().Value()
	stc, ok := stcInterface.(*StorageContext)
	if !ok {
		return fmt.Errorf("%T is not a StorageContext", stcInterface)
	}
	key := v.Estack().Pop().Bytes()
	value := v.Estack().Pop().Bytes()
	var flag int
	if getFlag {
		flag = int(v.Estack().Pop().BigInt().Int64())
	}
	return putWithContextAndFlags(ic, v, stc, key, value, flag == 1)
}

// storagePut puts key-value pair into the storage.
func storagePut(ic *interop.Context, v *vm.VM) error {
	return storagePutInternal(ic, v, false)
}

// storagePutEx puts key-value pair with given flags into the storage.
func storagePutEx(ic *interop.Context, v *vm.VM) error {
	return storagePutInternal(ic, v, true)
}

// storageContextAsReadOnly sets given context to read-only mode.
func storageContextAsReadOnly(ic *interop.Context, v *vm.VM) error {
	stcInterface := v.Estack().Pop().Value()
	stc, ok := stcInterface.(*StorageContext)
	if !ok {
		return fmt.Errorf("%T is not a StorageContext", stcInterface)
	}
	if !stc.ReadOnly {
		stx := &StorageContext{
			ID:       stc.ID,
			ReadOnly: true,
		}
		stc = stx
	}
	v.Estack().PushVal(stackitem.NewInterop(stc))
	return nil
}

// contractCall calls a contract.
func contractCall(ic *interop.Context, v *vm.VM) error {
	h := v.Estack().Pop().Bytes()
	method := v.Estack().Pop().Item()
	args := v.Estack().Pop().Item()
	return contractCallExInternal(ic, v, h, method, args, smartcontract.All)
}

// contractCallEx calls a contract with flags.
func contractCallEx(ic *interop.Context, v *vm.VM) error {
	h := v.Estack().Pop().Bytes()
	method := v.Estack().Pop().Item()
	args := v.Estack().Pop().Item()
	flags := smartcontract.CallFlag(int32(v.Estack().Pop().BigInt().Int64()))
	return contractCallExInternal(ic, v, h, method, args, flags)
}

func contractCallExInternal(ic *interop.Context, v *vm.VM, h []byte, method stackitem.Item, args stackitem.Item, f smartcontract.CallFlag) error {
	u, err := util.Uint160DecodeBytesBE(h)
	if err != nil {
		return errors.New("invalid contract hash")
	}
	cs, err := ic.DAO.GetContractState(u)
	if err != nil {
		return errors.New("contract not found")
	}
	bs, err := method.TryBytes()
	if err != nil {
		return err
	}
	curr, err := ic.DAO.GetContractState(v.GetCurrentScriptHash())
	if err == nil {
		if !curr.Manifest.CanCall(&cs.Manifest, string(bs)) {
			return errors.New("disallowed method call")
		}
	}
	ic.Invocations[u]++
	v.LoadScriptWithHash(cs.Script, u, v.Context().GetCallFlags()&f)
	v.Estack().PushVal(args)
	v.Estack().PushVal(method)
	return nil
}

// contractDestroy destroys a contract.
func contractDestroy(ic *interop.Context, v *vm.VM) error {
	hash := v.GetCurrentScriptHash()
	cs, err := ic.DAO.GetContractState(hash)
	if err != nil {
		return nil
	}
	err = ic.DAO.DeleteContractState(hash)
	if err != nil {
		return err
	}
	if cs.HasStorage() {
		siMap, err := ic.DAO.GetStorageItems(cs.ID)
		if err != nil {
			return err
		}
		for k := range siMap {
			_ = ic.DAO.DeleteStorageItem(cs.ID, []byte(k))
		}
	}
	return nil
}

// contractIsStandard checks if contract is standard (sig or multisig) contract.
func contractIsStandard(ic *interop.Context, v *vm.VM) error {
	h := v.Estack().Pop().Bytes()
	u, err := util.Uint160DecodeBytesBE(h)
	if err != nil {
		return err
	}
	var result bool
	cs, _ := ic.DAO.GetContractState(u)
	if cs == nil || vm.IsStandardContract(cs.Script) {
		result = true
	}
	v.Estack().PushVal(result)
	return nil
}

// contractCreateStandardAccount calculates contract scripthash for a given public key.
func contractCreateStandardAccount(ic *interop.Context, v *vm.VM) error {
	h := v.Estack().Pop().Bytes()
	p, err := keys.NewPublicKeyFromBytes(h)
	if err != nil {
		return err
	}
	v.Estack().PushVal(p.GetNEO3ScriptHash().BytesBE())
	return nil
}
