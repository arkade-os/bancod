package txmatch

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// pkt builds a *psbt.Packet wrapping an unsigned MsgTx for tests.
func pkt(t *testing.T, tx *wire.MsgTx) *psbt.Packet {
	t.Helper()
	p, err := psbt.NewFromUnsignedTx(tx)
	require.NoError(t, err)
	return p
}

func mkTx(outs []*wire.TxOut, ins []*wire.TxIn, lock uint32) *wire.MsgTx {
	tx := wire.NewMsgTx(2)
	tx.LockTime = lock
	for _, o := range outs {
		tx.AddTxOut(o)
	}
	for _, in := range ins {
		tx.AddTxIn(in)
	}
	return tx
}

func TestComposition(t *testing.T) {
	tx := pkt(t, mkTx(nil, nil, 0))
	require.True(t, Always(true)(tx))
	require.False(t, Always(false)(tx))

	require.True(t, All()(tx))  // empty AND is true
	require.False(t, Any()(tx)) // empty OR is false
	require.False(t, Not(Always(true))(tx))
	require.True(t, Not(Always(false))(tx))

	require.True(t, All(Always(true), Always(true))(tx))
	require.False(t, All(Always(true), Always(false))(tx))
	require.True(t, Any(Always(false), Always(true))(tx))
	require.False(t, Any(Always(false), Always(false))(tx))
}

func TestNilTxIsFalse(t *testing.T) {
	predicates := []Predicate{
		HasOutputCount(0),
		HasInputCount(0),
		HasLockTime(0),
		HasOutput(),
		HasInput(),
		HasUnknownGlobalField([]byte("k")),
	}
	for _, p := range predicates {
		require.False(t, p(nil))
	}
}

func TestHasOutputCount(t *testing.T) {
	tx := pkt(t, mkTx([]*wire.TxOut{
		{Value: 100, PkScript: []byte{0x01}},
		{Value: 200, PkScript: []byte{0x02}},
	}, nil, 0))
	require.True(t, HasOutputCount(2)(tx))
	require.False(t, HasOutputCount(1)(tx))
	require.False(t, HasOutputCount(3)(tx))
}

func TestHasInputCount(t *testing.T) {
	tx := pkt(t, mkTx(
		[]*wire.TxOut{{Value: 1, PkScript: []byte{0x00}}},
		[]*wire.TxIn{{PreviousOutPoint: wire.OutPoint{Index: 0}}},
		0,
	))
	require.True(t, HasInputCount(1)(tx))
	require.False(t, HasInputCount(0)(tx))
}

func TestHasLockTime(t *testing.T) {
	tx := pkt(t, mkTx(nil, nil, 42))
	require.True(t, HasLockTime(42)(tx))
	require.False(t, HasLockTime(0)(tx))
}

func TestHasUnknownGlobalField(t *testing.T) {
	tx := pkt(t, mkTx(nil, nil, 0))
	tx.Unknowns = append(tx.Unknowns, &psbt.Unknown{Key: []byte("foo"), Value: []byte("bar")})
	require.True(t, HasUnknownGlobalField([]byte("foo"))(tx))
	require.False(t, HasUnknownGlobalField([]byte("baz"))(tx))
}

func TestHasOutput_NoOpts(t *testing.T) {
	empty := pkt(t, mkTx(nil, nil, 0))
	require.False(t, HasOutput()(empty))

	with := pkt(t, mkTx([]*wire.TxOut{{Value: 1, PkScript: []byte{0x00}}}, nil, 0))
	require.True(t, HasOutput()(with))
}

func TestHasOutput_AmountVariants(t *testing.T) {
	tx := pkt(t, mkTx([]*wire.TxOut{
		{Value: 100, PkScript: []byte{0x01}},
		{Value: 500, PkScript: []byte{0x02}},
	}, nil, 0))

	require.True(t, HasOutput(OutputAmount(500))(tx))
	require.False(t, HasOutput(OutputAmount(999))(tx))

	require.True(t, HasOutput(OutputAmountAtLeast(500))(tx))
	require.False(t, HasOutput(OutputAmountAtLeast(501))(tx))

	require.True(t, HasOutput(OutputAmountAtMost(100))(tx))
	require.False(t, HasOutput(OutputAmountAtMost(99))(tx))

	require.True(t, HasOutput(OutputAmountInRange(100, 500))(tx))
	require.False(t, HasOutput(OutputAmountInRange(600, 700))(tx))
}

func TestHasOutput_ScriptAndIndex(t *testing.T) {
	scriptA := []byte{0xaa, 0xbb}
	scriptB := []byte{0xcc, 0xdd}
	tx := pkt(t, mkTx([]*wire.TxOut{
		{Value: 1, PkScript: scriptA},
		{Value: 2, PkScript: scriptB},
	}, nil, 0))

	require.True(t, HasOutput(OutputScript(scriptA))(tx))
	require.True(t, HasOutput(OutputScript(scriptB))(tx))
	require.False(t, HasOutput(OutputScript([]byte{0x00}))(tx))

	// OutputIndex constrains the search.
	require.True(t, HasOutput(OutputIndex(0), OutputScript(scriptA))(tx))
	require.False(t, HasOutput(OutputIndex(0), OutputScript(scriptB))(tx))
	require.False(t, HasOutput(OutputIndex(99))(tx))
	require.False(t, HasOutput(OutputIndex(-1))(tx))

	// ScriptMatching with predicate
	hasPrefix := func(prefix byte) func([]byte) bool {
		return func(s []byte) bool { return len(s) > 0 && s[0] == prefix }
	}
	require.True(t, HasOutput(OutputScriptMatching(hasPrefix(0xcc)))(tx))
	require.False(t, HasOutput(OutputScriptMatching(hasPrefix(0xff)))(tx))
}

func TestHasOutput_CombinedCriteria(t *testing.T) {
	scriptA := []byte{0xaa}
	tx := pkt(t, mkTx([]*wire.TxOut{
		{Value: 100, PkScript: scriptA},
		{Value: 999, PkScript: []byte{0xbb}},
	}, nil, 0))
	// Index 0 has both amount=100 and script=scriptA → true
	require.True(t, HasOutput(OutputIndex(0), OutputAmount(100), OutputScript(scriptA))(tx))
	// Index 1 has script=0xbb, asking for scriptA → false
	require.False(t, HasOutput(OutputIndex(1), OutputScript(scriptA))(tx))
}

func TestHasInput(t *testing.T) {
	hash, err := chainhash.NewHashFromStr("1111111111111111111111111111111111111111111111111111111111111111")
	require.NoError(t, err)
	op := wire.OutPoint{Hash: *hash, Index: 7}
	tx := pkt(t, mkTx(
		[]*wire.TxOut{{Value: 1, PkScript: []byte{0x00}}},
		[]*wire.TxIn{
			{PreviousOutPoint: op, Sequence: 0xfffffffd},
			{PreviousOutPoint: wire.OutPoint{Index: 0}, Sequence: 0},
		},
		0,
	))

	require.True(t, HasInput(InputSpending(op))(tx))
	require.True(t, HasInput(InputIndex(0), InputSpending(op))(tx))
	require.False(t, HasInput(InputIndex(1), InputSpending(op))(tx))
	require.True(t, HasInput(InputSequence(0xfffffffd))(tx))
	require.False(t, HasInput(InputSequence(0xdeadbeef))(tx))
	require.False(t, HasInput(InputIndex(99))(tx))
}
