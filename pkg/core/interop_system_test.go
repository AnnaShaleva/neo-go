package core

import (
	"math/big"
	"testing"

	"github.com/nspcc-dev/dbft/crypto"
	"github.com/nspcc-dev/neo-go/pkg/config/netmode"
	"github.com/nspcc-dev/neo-go/pkg/core/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/hash"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/opcode"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/stretchr/testify/require"
)

func TestBCGetTransaction(t *testing.T) {
	v, tx, context, chain := createVMAndTX(t)
	defer chain.Close()

	t.Run("success", func(t *testing.T) {
		require.NoError(t, context.DAO.StoreAsTransaction(tx, 0))
		v.Estack().PushVal(tx.Hash().BytesBE())
		err := bcGetTransaction(context, v)
		require.NoError(t, err)

		value := v.Estack().Pop().Value()
		actual, ok := value.([]stackitem.Item)
		require.True(t, ok)
		require.Equal(t, 8, len(actual))
		require.Equal(t, tx.Hash().BytesBE(), actual[0].Value().([]byte))
		require.Equal(t, int64(tx.Version), actual[1].Value().(*big.Int).Int64())
		require.Equal(t, int64(tx.Nonce), actual[2].Value().(*big.Int).Int64())
		require.Equal(t, tx.Sender.BytesBE(), actual[3].Value().([]byte))
		require.Equal(t, int64(tx.SystemFee), actual[4].Value().(*big.Int).Int64())
		require.Equal(t, int64(tx.NetworkFee), actual[5].Value().(*big.Int).Int64())
		require.Equal(t, int64(tx.ValidUntilBlock), actual[6].Value().(*big.Int).Int64())
		require.Equal(t, tx.Script, actual[7].Value().([]byte))
	})

	t.Run("isn't traceable", func(t *testing.T) {
		require.NoError(t, context.DAO.StoreAsTransaction(tx, 1))
		v.Estack().PushVal(tx.Hash().BytesBE())
		err := bcGetTransaction(context, v)
		require.NoError(t, err)

		_, ok := v.Estack().Pop().Item().(stackitem.Null)
		require.True(t, ok)
	})

	t.Run("bad hash", func(t *testing.T) {
		require.NoError(t, context.DAO.StoreAsTransaction(tx, 1))
		v.Estack().PushVal(tx.Hash().BytesLE())
		err := bcGetTransaction(context, v)
		require.NoError(t, err)

		_, ok := v.Estack().Pop().Item().(stackitem.Null)
		require.True(t, ok)
	})
}

func TestBCGetTransactionFromBlock(t *testing.T) {
	v, block, context, chain := createVMAndBlock(t)
	defer chain.Close()
	require.NoError(t, chain.AddBlock(chain.newBlock()))
	require.NoError(t, context.DAO.StoreAsBlock(block))

	t.Run("success", func(t *testing.T) {
		v.Estack().PushVal(0)
		v.Estack().PushVal(block.Hash().BytesBE())
		err := bcGetTransactionFromBlock(context, v)
		require.NoError(t, err)

		value := v.Estack().Pop().Value()
		actual, ok := value.([]byte)
		require.True(t, ok)
		require.Equal(t, block.Transactions[0].Hash().BytesBE(), actual)
	})

	t.Run("invalid block hash", func(t *testing.T) {
		v.Estack().PushVal(0)
		v.Estack().PushVal(block.Hash().BytesBE()[:10])
		err := bcGetTransactionFromBlock(context, v)
		require.Error(t, err)
	})

	t.Run("isn't traceable", func(t *testing.T) {
		block.Index = 2
		require.NoError(t, context.DAO.StoreAsBlock(block))
		v.Estack().PushVal(0)
		v.Estack().PushVal(block.Hash().BytesBE())
		err := bcGetTransactionFromBlock(context, v)
		require.NoError(t, err)

		_, ok := v.Estack().Pop().Item().(stackitem.Null)
		require.True(t, ok)
	})

	t.Run("bad block hash", func(t *testing.T) {
		block.Index = 1
		require.NoError(t, context.DAO.StoreAsBlock(block))
		v.Estack().PushVal(0)
		v.Estack().PushVal(block.Hash().BytesLE())
		err := bcGetTransactionFromBlock(context, v)
		require.NoError(t, err)

		_, ok := v.Estack().Pop().Item().(stackitem.Null)
		require.True(t, ok)
	})

	t.Run("bad transaction index", func(t *testing.T) {
		require.NoError(t, context.DAO.StoreAsBlock(block))
		v.Estack().PushVal(1)
		v.Estack().PushVal(block.Hash().BytesBE())
		err := bcGetTransactionFromBlock(context, v)
		require.Error(t, err)
	})
}

func TestBCGetBlock(t *testing.T) {
	v, context, chain := createVM(t)
	defer chain.Close()
	block := chain.newBlock()
	require.NoError(t, chain.AddBlock(block))

	t.Run("success", func(t *testing.T) {
		v.Estack().PushVal(block.Hash().BytesBE())
		err := bcGetBlock(context, v)
		require.NoError(t, err)

		value := v.Estack().Pop().Value()
		actual, ok := value.([]stackitem.Item)
		require.True(t, ok)
		require.Equal(t, 8, len(actual))
		require.Equal(t, block.Hash().BytesBE(), actual[0].Value().([]byte))
		require.Equal(t, int64(block.Version), actual[1].Value().(*big.Int).Int64())
		require.Equal(t, block.PrevHash.BytesBE(), actual[2].Value().([]byte))
		require.Equal(t, block.MerkleRoot.BytesBE(), actual[3].Value().([]byte))
		require.Equal(t, int64(block.Timestamp), actual[4].Value().(*big.Int).Int64())
		require.Equal(t, int64(block.Index), actual[5].Value().(*big.Int).Int64())
		require.Equal(t, block.NextConsensus.BytesBE(), actual[6].Value().([]byte))
		require.Equal(t, int64(len(block.Transactions)), actual[7].Value().(*big.Int).Int64())
	})

	t.Run("bad hash", func(t *testing.T) {
		v.Estack().PushVal(block.Hash().BytesLE())
		err := bcGetTransaction(context, v)
		require.NoError(t, err)

		_, ok := v.Estack().Pop().Item().(stackitem.Null)
		require.True(t, ok)
	})
}

func TestContractIsStandard(t *testing.T) {
	v, ic, chain := createVM(t)
	defer chain.Close()

	t.Run("contract not stored", func(t *testing.T) {
		priv, err := keys.NewPrivateKey()
		require.NoError(t, err)

		pub := priv.PublicKey()
		tx := transaction.New(netmode.TestNet, []byte{1, 2, 3}, 1)
		tx.Scripts = []transaction.Witness{
			{
				InvocationScript:   []byte{1, 2, 3},
				VerificationScript: pub.GetVerificationScript(),
			},
		}
		ic.Container = tx

		t.Run("true", func(t *testing.T) {
			v.Estack().PushVal(pub.GetScriptHash().BytesBE())
			require.NoError(t, contractIsStandard(ic, v))
			require.True(t, v.Estack().Pop().Bool())
		})

		t.Run("false", func(t *testing.T) {
			tx.Scripts[0].VerificationScript = []byte{9, 8, 7}
			v.Estack().PushVal(pub.GetScriptHash().BytesBE())
			require.NoError(t, contractIsStandard(ic, v))
			require.False(t, v.Estack().Pop().Bool())
		})
	})

	t.Run("contract stored, true", func(t *testing.T) {
		priv, err := keys.NewPrivateKey()
		require.NoError(t, err)

		pub := priv.PublicKey()
		err = ic.DAO.PutContractState(&state.Contract{ID: 42, Script: pub.GetVerificationScript()})
		require.NoError(t, err)

		v.Estack().PushVal(pub.GetScriptHash().BytesBE())
		require.NoError(t, contractIsStandard(ic, v))
		require.True(t, v.Estack().Pop().Bool())
	})
	t.Run("contract stored, false", func(t *testing.T) {
		script := []byte{byte(opcode.PUSHT)}
		require.NoError(t, ic.DAO.PutContractState(&state.Contract{ID: 24, Script: script}))

		v.Estack().PushVal(crypto.Hash160(script).BytesBE())
		require.NoError(t, contractIsStandard(ic, v))
		require.False(t, v.Estack().Pop().Bool())
	})
}

func TestContractCreateAccount(t *testing.T) {
	v, ic, chain := createVM(t)
	defer chain.Close()
	t.Run("Good", func(t *testing.T) {
		priv, err := keys.NewPrivateKey()
		require.NoError(t, err)
		pub := priv.PublicKey()
		v.Estack().PushVal(pub.Bytes())
		require.NoError(t, contractCreateStandardAccount(ic, v))

		value := v.Estack().Pop().Bytes()
		u, err := util.Uint160DecodeBytesBE(value)
		require.NoError(t, err)
		require.Equal(t, pub.GetScriptHash(), u)
	})
	t.Run("InvalidKey", func(t *testing.T) {
		v.Estack().PushVal([]byte{1, 2, 3})
		require.Error(t, contractCreateStandardAccount(ic, v))
	})
}

func TestContractCall(t *testing.T) {
	script := []byte{byte(opcode.DROP), byte(opcode.UNPACK), byte(opcode.DROP), byte(opcode.ADD)}
	h := hash.Hash160(script)
	m := manifest.NewManifest(h)
	m.ABI.EntryPoint.Parameters = []manifest.Parameter{
		manifest.NewParameter("Operation", smartcontract.StringType),
		manifest.NewParameter("Arguments", smartcontract.IntegerType),
	}
	m.ABI.EntryPoint.ReturnType = smartcontract.IntegerType
	cs := &state.Contract{
		Script:   script,
		Manifest: *m,
		ID:       123,
	}

	t.Run("2Arguments", func(t *testing.T) {
		v, ic, chain := createVM(t)
		defer chain.Close()

		require.NoError(t, ic.DAO.PutContractState(cs))

		v.LoadScriptWithFlags([]byte{byte(opcode.NOP)}, smartcontract.AllowCall)
		v.Estack().PushVal(42) // canary
		v.Estack().PushVal(stackitem.NewArray([]stackitem.Item{stackitem.Make(1), stackitem.Make(2)}))
		v.Estack().PushVal("add")
		v.Estack().PushVal(h.BytesBE())
		require.NoError(t, contractCall(ic, v))
		require.NoError(t, v.Run())
		require.Equal(t, 2, v.Estack().Len())
		require.Equal(t, big.NewInt(3), v.Estack().Pop().Value())
		require.Equal(t, big.NewInt(42), v.Estack().Pop().Value())
	})

	t.Run("1Argument", func(t *testing.T) {
		v, ic, chain := createVM(t)
		defer chain.Close()

		require.NoError(t, ic.DAO.PutContractState(cs))

		v.LoadScriptWithFlags([]byte{byte(opcode.NOP)}, smartcontract.AllowCall)
		v.Estack().PushVal(42) // canary
		v.Estack().PushVal(stackitem.NewArray([]stackitem.Item{stackitem.Make(1)}))
		v.Estack().PushVal("add")
		v.Estack().PushVal(h.BytesBE())
		require.NoError(t, contractCall(ic, v))
		require.Error(t, v.Run())
	})
}

func TestRuntimeGasLeft(t *testing.T) {
	v, ic, chain := createVM(t)
	defer chain.Close()

	v.GasLimit = 100
	v.AddGas(58)
	require.NoError(t, runtime.GasLeft(ic, v))
	require.EqualValues(t, 42, v.Estack().Pop().BigInt().Int64())
}

func TestRuntimeGetNotifications(t *testing.T) {
	v, ic, chain := createVM(t)
	defer chain.Close()

	ic.Notifications = []state.NotificationEvent{
		{ScriptHash: util.Uint160{1}, Name: "Event1", Item: stackitem.NewArray([]stackitem.Item{stackitem.NewByteArray([]byte{11})})},
		{ScriptHash: util.Uint160{2}, Name: "Event2", Item: stackitem.NewArray([]stackitem.Item{stackitem.NewByteArray([]byte{22})})},
		{ScriptHash: util.Uint160{1}, Name: "Event1", Item: stackitem.NewArray([]stackitem.Item{stackitem.NewByteArray([]byte{33})})},
	}

	t.Run("NoFilter", func(t *testing.T) {
		v.Estack().PushVal(stackitem.Null{})
		require.NoError(t, runtime.GetNotifications(ic, v))

		arr := v.Estack().Pop().Array()
		require.Equal(t, len(ic.Notifications), len(arr))
		for i := range arr {
			elem := arr[i].Value().([]stackitem.Item)
			require.Equal(t, ic.Notifications[i].ScriptHash.BytesBE(), elem[0].Value())
			name, err := elem[1].TryBytes()
			require.NoError(t, err)
			require.Equal(t, ic.Notifications[i].Name, string(name))
			require.Equal(t, ic.Notifications[i].Item, elem[2])
		}
	})

	t.Run("WithFilter", func(t *testing.T) {
		h := util.Uint160{2}.BytesBE()
		v.Estack().PushVal(h)
		require.NoError(t, runtime.GetNotifications(ic, v))

		arr := v.Estack().Pop().Array()
		require.Equal(t, 1, len(arr))
		elem := arr[0].Value().([]stackitem.Item)
		require.Equal(t, h, elem[0].Value())
		name, err := elem[1].TryBytes()
		require.NoError(t, err)
		require.Equal(t, ic.Notifications[1].Name, string(name))
		require.Equal(t, ic.Notifications[1].Item, elem[2])
	})
}

func TestRuntimeGetInvocationCounter(t *testing.T) {
	v, ic, chain := createVM(t)
	defer chain.Close()

	ic.Invocations[hash.Hash160([]byte{2})] = 42

	t.Run("Zero", func(t *testing.T) {
		v.LoadScript([]byte{1})
		require.Error(t, runtime.GetInvocationCounter(ic, v))
	})
	t.Run("NonZero", func(t *testing.T) {
		v.LoadScript([]byte{2})
		require.NoError(t, runtime.GetInvocationCounter(ic, v))
		require.EqualValues(t, 42, v.Estack().Pop().BigInt().Int64())
	})
}

func TestBlockchainGetContractState(t *testing.T) {
	v, cs, ic, bc := createVMAndContractState(t)
	defer bc.Close()
	require.NoError(t, ic.DAO.PutContractState(cs))

	t.Run("positive", func(t *testing.T) {
		v.Estack().PushVal(cs.ScriptHash().BytesBE())
		require.NoError(t, bcGetContract(ic, v))

		actual := v.Estack().Pop().Item()
		compareContractStates(t, cs, actual)
	})

	t.Run("uncknown contract state", func(t *testing.T) {
		v.Estack().PushVal(util.Uint160{1, 2, 3}.BytesBE())
		require.NoError(t, bcGetContract(ic, v))

		actual := v.Estack().Pop().Item()
		require.Equal(t, stackitem.Null{}, actual)
	})
}

func TestContractCreate(t *testing.T) {
	v, cs, ic, bc := createVMAndContractState(t)
	v.GasLimit = -1
	defer bc.Close()

	putArgsOnStack := func() {
		manifest, err := cs.Manifest.MarshalJSON()
		require.NoError(t, err)
		v.Estack().PushVal(manifest)
		v.Estack().PushVal(cs.Script)
	}

	t.Run("positive", func(t *testing.T) {
		putArgsOnStack()

		require.NoError(t, contractCreate(ic, v))
		actual := v.Estack().Pop().Item()
		compareContractStates(t, cs, actual)
	})

	t.Run("invalid scripthash", func(t *testing.T) {
		cs.Script = append(cs.Script, 0x01)
		putArgsOnStack()

		require.Error(t, contractCreate(ic, v))
	})

	t.Run("contract already exists", func(t *testing.T) {
		cs.Script = cs.Script[:len(cs.Script)-1]
		require.NoError(t, ic.DAO.PutContractState(cs))
		putArgsOnStack()

		require.Error(t, contractCreate(ic, v))
	})
}

func compareContractStates(t *testing.T, expected *state.Contract, actual stackitem.Item) {
	act, ok := actual.Value().([]stackitem.Item)
	require.True(t, ok)

	expectedManifest, err := expected.Manifest.MarshalJSON()
	require.NoError(t, err)

	require.Equal(t, 4, len(act))
	require.Equal(t, expected.Script, act[0].Value().([]byte))
	require.Equal(t, expectedManifest, act[1].Value().([]byte))
	require.Equal(t, expected.HasStorage(), act[2].Bool())
	require.Equal(t, expected.IsPayable(), act[3].Bool())
}

func TestContractUpdate(t *testing.T) {
	v, cs, ic, bc := createVMAndContractState(t)
	defer bc.Close()
	v.GasLimit = -1

	putArgsOnStack := func(script, manifest []byte) {
		v.Estack().PushVal(manifest)
		v.Estack().PushVal(script)
	}

	t.Run("no args", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		putArgsOnStack(nil, nil)
		require.NoError(t, contractUpdate(ic, v))
	})

	t.Run("no contract", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, util.Uint160{8, 9, 7}, smartcontract.All)
		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("too large script", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		putArgsOnStack(make([]byte, MaxContractScriptSize+1), nil)
		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("too large manifest", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		putArgsOnStack(nil, make([]byte, manifest.MaxManifestSize+1))
		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("gas limit exceeded", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.GasLimit = 0
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		putArgsOnStack([]byte{1}, []byte{2})
		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("update script, the same script", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.GasLimit = -1
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		putArgsOnStack(cs.Script, nil)

		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("update script, already exists", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		duplicateScript := []byte{byte(opcode.PUSHDATA4)}
		require.NoError(t, ic.DAO.PutContractState(&state.Contract{
			ID:     95,
			Script: duplicateScript,
			Manifest: manifest.Manifest{
				ABI: manifest.ABI{
					Hash: hash.Hash160(duplicateScript),
				},
			},
		}))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		putArgsOnStack(duplicateScript, nil)

		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("update script, positive", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		newScript := []byte{9, 8, 7, 6, 5}
		putArgsOnStack(newScript, nil)

		require.NoError(t, contractUpdate(ic, v))

		// updated contract should have new scripthash
		actual, err := ic.DAO.GetContractState(hash.Hash160(newScript))
		require.NoError(t, err)
		expected := &state.Contract{
			ID:       cs.ID,
			Script:   newScript,
			Manifest: cs.Manifest,
		}
		expected.Manifest.ABI.Hash = hash.Hash160(newScript)
		_ = expected.ScriptHash()
		require.Equal(t, expected, actual)

		// old contract should be deleted
		_, err = ic.DAO.GetContractState(cs.ScriptHash())
		require.Error(t, err)
	})

	t.Run("update manifest, bad manifest", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		putArgsOnStack(nil, []byte{1, 2, 3})

		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("update manifest, bad contract hash", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		manifest := &manifest.Manifest{
			ABI: manifest.ABI{
				Hash: util.Uint160{4, 5, 6},
			},
		}
		manifestBytes, err := manifest.MarshalJSON()
		require.NoError(t, err)
		putArgsOnStack(nil, manifestBytes)

		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("update manifest, old contract shouldn't have storage", func(t *testing.T) {
		cs.Manifest.Features |= smartcontract.HasStorage
		require.NoError(t, ic.DAO.PutContractState(cs))
		require.NoError(t, ic.DAO.PutStorageItem(cs.ID, []byte("my_item"), &state.StorageItem{
			Value:   []byte{1, 2, 3},
			IsConst: false,
		}))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		manifest := &manifest.Manifest{
			ABI: manifest.ABI{
				Hash: cs.ScriptHash(),
			},
		}
		manifestBytes, err := manifest.MarshalJSON()
		require.NoError(t, err)
		putArgsOnStack(nil, manifestBytes)

		require.Error(t, contractUpdate(ic, v))
	})

	t.Run("update manifest, positive", func(t *testing.T) {
		cs.Manifest.Features = smartcontract.NoProperties
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		manifest := &manifest.Manifest{
			ABI: manifest.ABI{
				Hash: cs.ScriptHash(),
				EntryPoint: manifest.Method{
					Name: "Main",
					Parameters: []manifest.Parameter{
						manifest.NewParameter("NewParameter", smartcontract.IntegerType),
					},
					ReturnType: smartcontract.StringType,
				},
			},
			Features: smartcontract.HasStorage,
		}
		manifestBytes, err := manifest.MarshalJSON()
		require.NoError(t, err)
		putArgsOnStack(nil, manifestBytes)

		require.NoError(t, contractUpdate(ic, v))

		// updated contract should have new scripthash
		actual, err := ic.DAO.GetContractState(cs.ScriptHash())
		expected := &state.Contract{
			ID:       cs.ID,
			Script:   cs.Script,
			Manifest: *manifest,
		}
		_ = expected.ScriptHash()
		require.Equal(t, expected, actual)
	})

	t.Run("update both script and manifest", func(t *testing.T) {
		require.NoError(t, ic.DAO.PutContractState(cs))
		v.LoadScriptWithHash([]byte{byte(opcode.RET)}, cs.ScriptHash(), smartcontract.All)
		newScript := []byte{12, 13, 14}
		newManifest := manifest.Manifest{
			ABI: manifest.ABI{
				Hash: hash.Hash160(newScript),
				EntryPoint: manifest.Method{
					Name: "Main",
					Parameters: []manifest.Parameter{
						manifest.NewParameter("VeryNewParameter", smartcontract.IntegerType),
					},
					ReturnType: smartcontract.StringType,
				},
			},
			Features: smartcontract.HasStorage,
		}
		newManifestBytes, err := newManifest.MarshalJSON()
		require.NoError(t, err)

		putArgsOnStack(newScript, newManifestBytes)

		require.NoError(t, contractUpdate(ic, v))

		// updated contract should have new script and manifest
		actual, err := ic.DAO.GetContractState(hash.Hash160(newScript))
		require.NoError(t, err)
		expected := &state.Contract{
			ID:       cs.ID,
			Script:   newScript,
			Manifest: newManifest,
		}
		expected.Manifest.ABI.Hash = hash.Hash160(newScript)
		_ = expected.ScriptHash()
		require.Equal(t, expected, actual)

		// old contract should be deleted
		_, err = ic.DAO.GetContractState(cs.ScriptHash())
		require.Error(t, err)
	})
}

func TestContractGetCallFlags(t *testing.T) {
	v, ic, bc := createVM(t)
	defer bc.Close()

	v.LoadScriptWithHash([]byte{byte(opcode.RET)}, util.Uint160{1, 2, 3}, smartcontract.All)
	require.NoError(t, contractGetCallFlags(ic, v))
	require.Equal(t, int64(smartcontract.All), v.Estack().Pop().Value().(*big.Int).Int64())
}
