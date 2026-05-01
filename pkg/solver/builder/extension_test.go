package builder

import (
	"context"
	"testing"

	"github.com/arkade-os/arkd/pkg/ark-lib/extension"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

const tagFoo uint8 = 0x42

// txWithExtension builds a *psbt.Packet whose unsigned tx carries an ark
// extension OP_RETURN output (plus a dummy spendable output so the tx is
// otherwise valid).
func txWithExtension(t *testing.T, pkts ...extension.Packet) *psbt.Packet {
	t.Helper()
	ext, err := extension.NewExtensionFromPackets(pkts...)
	require.NoError(t, err)
	extOut, err := ext.TxOut()
	require.NoError(t, err)

	tx := wire.NewMsgTx(2)
	tx.AddTxOut(&wire.TxOut{Value: 1000, PkScript: []byte{0x51}}) // dummy
	tx.AddTxOut(extOut)
	p, err := psbt.NewFromUnsignedTx(tx)
	require.NoError(t, err)
	return p
}

func txWithoutExtension(t *testing.T) *psbt.Packet {
	t.Helper()
	tx := wire.NewMsgTx(2)
	tx.AddTxOut(&wire.TxOut{Value: 1000, PkScript: []byte{0x51}})
	p, err := psbt.NewFromUnsignedTx(tx)
	require.NoError(t, err)
	return p
}

func TestHasArkExtension(t *testing.T) {
	pred := HasArkExtension()
	require.False(t, pred(txWithoutExtension(t)), "no ext output -> no match")

	pkt := extension.UnknownPacket{PacketType: tagFoo, Data: []byte{1, 2, 3}}
	require.True(t, pred(txWithExtension(t, pkt)), "ext output -> match")
}

func TestForExtension_NoExtensionSkips(t *testing.T) {
	calls := 0
	plugin := ForExtension(func(context.Context, *psbt.Packet, extension.Extension) (*intent, error) {
		calls++
		return &intent{1}, nil
	}).Solve(func(context.Context, *intent) {}).Build()

	_, ok := plugin.Match(context.Background(), txWithoutExtension(t))
	require.False(t, ok)
	require.Equal(t, 0, calls, "decoder must not run when there's no extension")
}

func TestForExtension_HandsParsedExtensionToDecoder(t *testing.T) {
	pkt := extension.UnknownPacket{PacketType: tagFoo, Data: []byte{0xab, 0xcd}}
	var got extension.Extension
	plugin := ForExtension(func(_ context.Context, _ *psbt.Packet, ext extension.Extension) (*intent, error) {
		got = ext
		return &intent{value: int(ext[0].Type())}, nil
	}).Solve(func(context.Context, *intent) {}).Build()

	out, ok := plugin.Match(context.Background(), txWithExtension(t, pkt))
	require.True(t, ok)
	require.NotNil(t, got)
	require.Equal(t, int(tagFoo), out.(*intent).value)
}

func TestForExtension_DecoderCanReturnSkip(t *testing.T) {
	pkt := extension.UnknownPacket{PacketType: tagFoo, Data: []byte{1}}
	plugin := ForExtension(func(context.Context, *psbt.Packet, extension.Extension) (*intent, error) {
		return nil, ErrSkip
	}).Solve(func(context.Context, *intent) {}).Build()

	_, ok := plugin.Match(context.Background(), txWithExtension(t, pkt))
	require.False(t, ok)
}

func TestForExtension_NilDecoderPanics(t *testing.T) {
	require.PanicsWithValue(t, "builder: ForExtension requires a decoder", func() {
		ForExtension[*intent](nil)
	})
}
