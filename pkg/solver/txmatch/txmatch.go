// Package txmatch provides composable, pure-function predicates over
// *psbt.Packet. Predicates are the structural-filter layer plugins use
// to decide "is this tx even worth decoding?" — they perform no I/O and
// hold no state, so they're cheap to run on every tx in the stream.
package txmatch

import (
	"bytes"

	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/wire"
)

// Predicate is a pure-function check over a parsed PSBT. A nil tx (or
// missing UnsignedTx) yields false from every constructor in this package.
type Predicate func(*psbt.Packet) bool

// Always returns a Predicate that ignores its input and returns v.
func Always(v bool) Predicate { return func(*psbt.Packet) bool { return v } }

// All returns a Predicate that is true iff every p is true (AND).
// An empty list is true.
func All(ps ...Predicate) Predicate {
	return func(tx *psbt.Packet) bool {
		for _, p := range ps {
			if p == nil || !p(tx) {
				return false
			}
		}
		return true
	}
}

// Any returns a Predicate that is true iff any p is true (OR).
// An empty list is false.
func Any(ps ...Predicate) Predicate {
	return func(tx *psbt.Packet) bool {
		for _, p := range ps {
			if p != nil && p(tx) {
				return true
			}
		}
		return false
	}
}

// Not negates p.
func Not(p Predicate) Predicate {
	return func(tx *psbt.Packet) bool { return p != nil && !p(tx) }
}

// HasOutputCount matches transactions with exactly n outputs.
func HasOutputCount(n int) Predicate {
	return func(tx *psbt.Packet) bool {
		if tx == nil || tx.UnsignedTx == nil {
			return false
		}
		return len(tx.UnsignedTx.TxOut) == n
	}
}

// HasInputCount matches transactions with exactly n inputs.
func HasInputCount(n int) Predicate {
	return func(tx *psbt.Packet) bool {
		if tx == nil || tx.UnsignedTx == nil {
			return false
		}
		return len(tx.UnsignedTx.TxIn) == n
	}
}

// HasLockTime matches transactions with the given nLockTime.
func HasLockTime(lt uint32) Predicate {
	return func(tx *psbt.Packet) bool {
		if tx == nil || tx.UnsignedTx == nil {
			return false
		}
		return tx.UnsignedTx.LockTime == lt
	}
}

// HasUnknownGlobalField matches PSBTs that carry an unknown global key-value
// with the given key. Useful as a cheap envelope check for protocols that
// stamp a tag into the global section.
func HasUnknownGlobalField(key []byte) Predicate {
	return func(tx *psbt.Packet) bool {
		if tx == nil {
			return false
		}
		for _, u := range tx.Unknowns {
			if u != nil && bytes.Equal(u.Key, key) {
				return true
			}
		}
		return false
	}
}

// outputCriteria is the accumulated state from OutputOpts, applied by HasOutput.
type outputCriteria struct {
	indexSet bool
	index    int
	amount   func(uint64) bool
	script   func([]byte) bool
}

// OutputOpt narrows which outputs HasOutput considers a hit.
type OutputOpt func(*outputCriteria)

// OutputIndex restricts the match to the output at the given index. Without
// this option, HasOutput searches all outputs and matches if any one passes.
func OutputIndex(i int) OutputOpt {
	return func(c *outputCriteria) { c.indexSet = true; c.index = i }
}

// OutputAmount matches outputs whose value (in sats) equals amount.
func OutputAmount(amount uint64) OutputOpt {
	return func(c *outputCriteria) {
		c.amount = func(v uint64) bool { return v == amount }
	}
}

// OutputAmountAtLeast matches outputs whose value is >= min sats.
func OutputAmountAtLeast(min uint64) OutputOpt {
	return func(c *outputCriteria) {
		c.amount = func(v uint64) bool { return v >= min }
	}
}

// OutputAmountAtMost matches outputs whose value is <= max sats.
func OutputAmountAtMost(max uint64) OutputOpt {
	return func(c *outputCriteria) {
		c.amount = func(v uint64) bool { return v <= max }
	}
}

// OutputAmountInRange matches outputs with min <= value <= max sats.
func OutputAmountInRange(min, max uint64) OutputOpt {
	return func(c *outputCriteria) {
		c.amount = func(v uint64) bool { return v >= min && v <= max }
	}
}

// OutputScript matches outputs whose pkScript equals script.
func OutputScript(script []byte) OutputOpt {
	want := append([]byte(nil), script...)
	return func(c *outputCriteria) {
		c.script = func(s []byte) bool { return bytes.Equal(s, want) }
	}
}

// OutputScriptMatching matches outputs whose pkScript satisfies f. Use for
// script-template checks (e.g. "is this an HTLC?", "is this a P2TR with
// internal key X?").
func OutputScriptMatching(f func([]byte) bool) OutputOpt {
	return func(c *outputCriteria) { c.script = f }
}

// HasOutput is true if any output (or the indexed output, if OutputIndex is
// given) satisfies all configured criteria. With no options it requires at
// least one output to exist.
func HasOutput(opts ...OutputOpt) Predicate {
	var c outputCriteria
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return func(tx *psbt.Packet) bool {
		if tx == nil || tx.UnsignedTx == nil {
			return false
		}
		outs := tx.UnsignedTx.TxOut
		if c.indexSet {
			if c.index < 0 || c.index >= len(outs) {
				return false
			}
			return matchOutput(outs[c.index], &c)
		}
		for _, o := range outs {
			if matchOutput(o, &c) {
				return true
			}
		}
		return false
	}
}

func matchOutput(o *wire.TxOut, c *outputCriteria) bool {
	if o == nil {
		return false
	}
	if c.amount != nil && !c.amount(uint64(o.Value)) {
		return false
	}
	if c.script != nil && !c.script(o.PkScript) {
		return false
	}
	return true
}

// inputCriteria is the accumulated state from InputOpts, applied by HasInput.
type inputCriteria struct {
	indexSet bool
	index    int
	prevOut  func(wire.OutPoint) bool
	sequence func(uint32) bool
}

// InputOpt narrows which inputs HasInput considers a hit.
type InputOpt func(*inputCriteria)

// InputIndex restricts the match to the input at the given index.
func InputIndex(i int) InputOpt {
	return func(c *inputCriteria) { c.indexSet = true; c.index = i }
}

// InputSpending matches inputs whose previous outpoint equals op.
func InputSpending(op wire.OutPoint) InputOpt {
	return func(c *inputCriteria) {
		c.prevOut = func(got wire.OutPoint) bool { return got == op }
	}
}

// InputSequence matches inputs with the given nSequence value.
func InputSequence(seq uint32) InputOpt {
	return func(c *inputCriteria) {
		c.sequence = func(got uint32) bool { return got == seq }
	}
}

// HasInput is true if any input (or the indexed input, if InputIndex is given)
// satisfies all configured criteria. With no options it requires at least one
// input to exist.
func HasInput(opts ...InputOpt) Predicate {
	var c inputCriteria
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return func(tx *psbt.Packet) bool {
		if tx == nil || tx.UnsignedTx == nil {
			return false
		}
		ins := tx.UnsignedTx.TxIn
		if c.indexSet {
			if c.index < 0 || c.index >= len(ins) {
				return false
			}
			return matchInput(ins[c.index], &c)
		}
		for _, in := range ins {
			if matchInput(in, &c) {
				return true
			}
		}
		return false
	}
}

func matchInput(in *wire.TxIn, c *inputCriteria) bool {
	if in == nil {
		return false
	}
	if c.prevOut != nil && !c.prevOut(in.PreviousOutPoint) {
		return false
	}
	if c.sequence != nil && !c.sequence(in.Sequence) {
		return false
	}
	return true
}
