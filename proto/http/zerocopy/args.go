// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bytes"
	"errors"
	"io"
	"iter"
	"sort"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"
)

const (
	ArgsNoValue  = true
	ArgsHasValue = false
)

var argsStorage = pool.NewPerPStorage(func() *Args {
	return &Args{}
})

// AcquireArgs returns an empty Args object from the pool.
//
// The returned Args may be returned to the pool with ReleaseArgs
// when no longer needed. This allows reducing GC load.
func AcquireArgs() *Args {
	return argsStorage.Get()
}

// ReleaseArgs returns the object acquired via AcquireArgs to the pool.
//
// Do not access the released Args object, otherwise data races may occur.
func ReleaseArgs(a *Args) {
	a.Reset()
	argsStorage.Put(a)
}

// Args represents query arguments.
//
// It is forbidden copying Args instances. Create new instances instead
// and use CopyTo().
//
// Args instance MUST NOT be used from concurrently running goroutines.
type Args struct {
	noCopy NoCopy

	args []ArgsKV
	buf  []byte
}

type ArgsKV struct {
	Key     []byte
	Value   []byte
	NoValue bool
}

// Reset clears query args.
func (a *Args) Reset() {
	a.args = a.args[:0]
}

// CopyTo copies all args to dst.
func (a *Args) CopyTo(dst *Args) {
	dst.args = CopyArgs(dst.args, a.args)
}

// All returns an iterator over key-value pairs from args.
//
// The key and value may invalid outside the iteration loop.
// Make copies if you need to use them after the loop ends.
//
// Making modifications to the Args during the iteration loop leads to undefined
// behavior and can cause panics.
func (a *Args) All() iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		for i := range a.args {
			if !yield(a.args[i].Key, a.args[i].Value) {
				break
			}
		}
	}
}

// Len returns the number of query args.
func (a *Args) Len() int {
	return len(a.args)
}

// Parse parses the given string containing query args.
func (a *Args) Parse(s string) {
	a.buf = append(a.buf[:0], s...)
	a.ParseBytes(a.buf)
}

// ParseBytes parses the given b containing query args.
func (a *Args) ParseBytes(b []byte) {
	a.Reset()

	var s argsScanner

	s.b = b

	var kv *ArgsKV

	a.args, kv = AllocArg(a.args)
	for s.next(kv) {
		if len(kv.Key) > 0 || len(kv.Value) > 0 {
			a.args, kv = AllocArg(a.args)
		}
	}

	a.args = ReleaseArg(a.args)
}

// String returns string representation of query args.
func (a *Args) String() string {
	return string(a.QueryString())
}

// QueryString returns query string for the args.
//
// The returned value is valid until the Args is reused or released (ReleaseArgs).
// Do not store references to the returned value. Make copies instead.
func (a *Args) QueryString() []byte {
	a.buf = a.AppendBytes(a.buf[:0])
	return a.buf
}

// Sort sorts Args by key and then value using 'f' as comparison function.
//
// For example args.Sort(bytes.Compare).
func (a *Args) Sort(f func(x, y []byte) int) {
	sort.SliceStable(a.args, func(i, j int) bool {
		n := f(a.args[i].Key, a.args[j].Key)
		if n == 0 {
			return f(a.args[i].Value, a.args[j].Value) == -1
		}

		return n == -1
	})
}

// SortKeys sorts Args by key only using 'f' as comparison function.
//
// For example args.SortKeys(bytes.Compare).
func (a *Args) SortKeys(f func(x, y []byte) int) {
	sort.SliceStable(a.args, func(i, j int) bool {
		return f(a.args[i].Key, a.args[j].Key) == -1
	})
}

// AppendBytes appends query string to dst and returns the extended dst.
func (a *Args) AppendBytes(dst []byte) []byte {
	args := a.args
	for i, n := 0, len(args); i < n; i++ {
		kv := &args[i]

		dst = AppendQuotedArg(dst, kv.Key)
		if !kv.NoValue {
			dst = append(dst, '=')
			if len(kv.Value) > 0 {
				dst = AppendQuotedArg(dst, kv.Value)
			}
		}

		if i+1 < n {
			dst = append(dst, '&')
		}
	}

	return dst
}

// WriteTo writes query string to w.
//
// WriteTo implements io.WriterTo interface.
func (a *Args) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(a.QueryString())
	return int64(n), err
}

// Del deletes argument with the given key from query args.
func (a *Args) Del(key string) {
	a.args = DelAllArgsStable(a.args, key)
}

// DelBytes deletes argument with the given key from query args.
func (a *Args) DelBytes(key []byte) {
	a.args = DelAllArgsStable(a.args, bytesconv.B2S(key))
}

// Add adds 'key=value' argument.
//
// Multiple values for the same key may be added.
func (a *Args) Add(key, value string) {
	a.args = AppendArg(a.args, key, value, ArgsHasValue)
}

// AddBytesK adds 'key=value' argument.
//
// Multiple values for the same key may be added.
func (a *Args) AddBytesK(key []byte, value string) {
	a.args = AppendArg(a.args, bytesconv.B2S(key), value, ArgsHasValue)
}

// AddBytesV adds 'key=value' argument.
//
// Multiple values for the same key may be added.
func (a *Args) AddBytesV(key string, value []byte) {
	a.args = AppendArg(a.args, key, bytesconv.B2S(value), ArgsHasValue)
}

// AddBytesKV adds 'key=value' argument.
//
// Multiple values for the same key may be added.
func (a *Args) AddBytesKV(key, value []byte) {
	a.args = AppendArg(a.args, bytesconv.B2S(key), bytesconv.B2S(value), ArgsHasValue)
}

// AddNoValue adds only 'key' as argument without the '='.
//
// Multiple values for the same key may be added.
func (a *Args) AddNoValue(key string) {
	a.args = AppendArg(a.args, key, "", ArgsNoValue)
}

// AddBytesKNoValue adds only 'key' as argument without the '='.
//
// Multiple values for the same key may be added.
func (a *Args) AddBytesKNoValue(key []byte) {
	a.args = AppendArg(a.args, bytesconv.B2S(key), "", ArgsNoValue)
}

// Set sets 'key=value' argument.
func (a *Args) Set(key, value string) {
	a.args = SetArg(a.args, key, value, ArgsHasValue)
}

// SetBytesK sets 'key=value' argument.
func (a *Args) SetBytesK(key []byte, value string) {
	a.args = SetArg(a.args, bytesconv.B2S(key), value, ArgsHasValue)
}

// SetBytesV sets 'key=value' argument.
func (a *Args) SetBytesV(key string, value []byte) {
	a.args = SetArg(a.args, key, bytesconv.B2S(value), ArgsHasValue)
}

// SetBytesKV sets 'key=value' argument.
func (a *Args) SetBytesKV(key, value []byte) {
	a.args = SetArgBytes(a.args, key, value, ArgsHasValue)
}

// SetNoValue sets only 'key' as argument without the '='.
//
// Only key in argument, like key1&key2.
func (a *Args) SetNoValue(key string) {
	a.args = SetArg(a.args, key, "", ArgsNoValue)
}

// SetBytesKNoValue sets 'key' argument.
func (a *Args) SetBytesKNoValue(key []byte) {
	a.args = SetArg(a.args, bytesconv.B2S(key), "", ArgsNoValue)
}

// Peek returns query arg value for the given key.
//
// The returned value is valid until the Args is reused or released (ReleaseArgs).
// Do not store references to the returned value. Make copies instead.
func (a *Args) Peek(key string) []byte {
	return PeekArgStr(a.args, key)
}

// PeekBytes returns query arg value for the given key.
//
// The returned value is valid until the Args is reused or released (ReleaseArgs).
// Do not store references to the returned value. Make copies instead.
func (a *Args) PeekBytes(key []byte) []byte {
	return PeekArgBytes(a.args, key)
}

// PeekMulti returns all the arg values for the given key.
func (a *Args) PeekMulti(key string) [][]byte {
	var values [][]byte
	for k, v := range a.All() {
		if string(k) == key {
			values = append(values, v)
		}
	}

	return values
}

// PeekMultiBytes returns all the arg values for the given key.
func (a *Args) PeekMultiBytes(key []byte) [][]byte {
	return a.PeekMulti(bytesconv.B2S(key))
}

// Has returns true if the given key exists in Args.
func (a *Args) Has(key string) bool {
	return hasArg(a.args, key)
}

// HasBytes returns true if the given key exists in Args.
func (a *Args) HasBytes(key []byte) bool {
	return hasArg(a.args, bytesconv.B2S(key))
}

// ErrNoArgValue is returned when Args value with the given key is missing.
var ErrNoArgValue = errors.New("zerocopy: no args value for the given key")

// GetUint returns uint value for the given key.
func (a *Args) GetUint(key string) (int, error) {
	value := a.Peek(key)
	if len(value) == 0 {
		return -1, ErrNoArgValue
	}

	return ParseUint(value)
}

// SetUint sets uint value for the given key.
func (a *Args) SetUint(key string, value int) {
	a.buf = AppendUint(a.buf[:0], value)
	a.SetBytesV(key, a.buf)
}

// SetUintBytes sets uint value for the given key.
func (a *Args) SetUintBytes(key []byte, value int) {
	a.SetUint(bytesconv.B2S(key), value)
}

// GetUintOrZero returns uint value for the given key.
//
// Zero (0) is returned on error.
func (a *Args) GetUintOrZero(key string) int {
	n, err := a.GetUint(key)
	if err != nil {
		n = 0
	}

	return n
}

// GetUfloat returns ufloat value for the given key.
func (a *Args) GetUfloat(key string) (float64, error) {
	value := a.Peek(key)
	if len(value) == 0 {
		return -1, ErrNoArgValue
	}

	return ParseUfloat(value)
}

// GetUfloatOrZero returns ufloat value for the given key.
//
// Zero (0) is returned on error.
func (a *Args) GetUfloatOrZero(key string) float64 {
	f, err := a.GetUfloat(key)
	if err != nil {
		f = 0
	}

	return f
}

// GetBool returns boolean value for the given key.
//
// true is returned for "1", "t", "T", "true", "TRUE", "True", "y", "yes", "Y", "YES", "Yes",
// otherwise false is returned.
func (a *Args) GetBool(key string) bool {
	switch string(a.Peek(key)) {
	// Support the same true cases as strconv.ParseBool
	// See: https://github.com/golang/go/blob/4e1b11e2c9bdb0ddea1141eed487be1a626ff5be/src/strconv/atob.go#L12
	// and Y and Yes versions.
	case "1", "t", "T", "true", "TRUE", "True", "y", "yes", "Y", "YES", "Yes":
		return true
	default:
		return false
	}
}

func CopyArgs(dst, src []ArgsKV) []ArgsKV {
	if cap(dst) < len(src) {
		tmp := make([]ArgsKV, len(src))
		dstLen := len(dst)
		dst = dst[:cap(dst)] // copy all of dst.
		copy(tmp, dst)

		for i := dstLen; i < len(tmp); i++ {
			// Make sure nothing is nil.
			tmp[i].Key = []byte{}
			tmp[i].Value = []byte{}
		}

		dst = tmp
	}

	n := len(src)

	dst = dst[:n]
	for i := range n {
		dstKV := &dst[i]
		srcKV := &src[i]

		dstKV.Key = append(dstKV.Key[:0], srcKV.Key...)
		if srcKV.NoValue {
			dstKV.Value = dstKV.Value[:0]
		} else {
			dstKV.Value = append(dstKV.Value[:0], srcKV.Value...)
		}

		dstKV.NoValue = srcKV.NoValue
	}

	return dst
}

func DelAllArgsStable(args []ArgsKV, key string) []ArgsKV {
	for i, n := 0, len(args); i < n; i++ {
		kv := &args[i]
		if key == string(kv.Key) {
			tmp := *kv

			copy(args[i:], args[i+1:])

			n--
			i--
			args[n] = tmp
			args = args[:n]
		}
	}

	return args
}

func DelAllArgs(args []ArgsKV, key string) []ArgsKV {
	n := len(args)
	for i := 0; i < n; i++ {
		if key == string(args[i].Key) {
			args[i], args[n-1] = args[n-1], args[i]
			n--
			i--
		}
	}

	return args[:n]
}

func SetArgBytes(h []ArgsKV, key, value []byte, noValue bool) []ArgsKV {
	return SetArg(h, bytesconv.B2S(key), bytesconv.B2S(value), noValue)
}

func SetArg(h []ArgsKV, key, value string, noValue bool) []ArgsKV {
	n := len(h)
	for i := range n {
		kv := &h[i]
		if key == string(kv.Key) {
			if noValue {
				kv.Value = kv.Value[:0]
			} else {
				kv.Value = append(kv.Value[:0], value...)
			}

			kv.NoValue = noValue

			return h
		}
	}

	return AppendArg(h, key, value, noValue)
}

//nolint:unused
func AppendArgBytes(h []ArgsKV, key, value []byte, noValue bool) []ArgsKV {
	return AppendArg(h, bytesconv.B2S(key), bytesconv.B2S(value), noValue)
}

func AppendArg(args []ArgsKV, key, value string, noValue bool) []ArgsKV {
	var kv *ArgsKV

	args, kv = AllocArg(args)

	kv.Key = append(kv.Key[:0], key...)
	if noValue {
		kv.Value = kv.Value[:0]
	} else {
		kv.Value = append(kv.Value[:0], value...)
	}

	kv.NoValue = noValue

	return args
}

func AllocArg(h []ArgsKV) ([]ArgsKV, *ArgsKV) {
	n := len(h)
	if cap(h) > n {
		h = h[:n+1]
	} else {
		h = append(h, ArgsKV{
			Value: []byte{},
		})
	}

	return h, &h[n]
}

func ReleaseArg(h []ArgsKV) []ArgsKV {
	return h[:len(h)-1]
}

func hasArg(h []ArgsKV, key string) bool {
	for i, n := 0, len(h); i < n; i++ {
		kv := &h[i]
		if key == string(kv.Key) {
			return true
		}
	}

	return false
}

func PeekArgBytes(h []ArgsKV, k []byte) []byte {
	for i, n := 0, len(h); i < n; i++ {
		kv := &h[i]
		if bytes.Equal(kv.Key, k) {
			return kv.Value
		}
	}

	return nil
}

func PeekArgStr(h []ArgsKV, k string) []byte {
	for i, n := 0, len(h); i < n; i++ {
		kv := &h[i]
		if string(kv.Key) == k {
			return kv.Value
		}
	}

	return nil
}

type argsScanner struct {
	b []byte
}

func (s *argsScanner) next(kv *ArgsKV) bool {
	if len(s.b) == 0 {
		return false
	}

	kv.NoValue = ArgsHasValue

	isKey := true

	k := 0
	for i, c := range s.b {
		switch c {
		case '=':
			if isKey {
				isKey = false
				kv.Key = DecodeArgAppend(kv.Key[:0], s.b[:i])
				k = i + 1
			}

		case '&':
			if isKey {
				kv.Key = DecodeArgAppend(kv.Key[:0], s.b[:i])
				kv.Value = kv.Value[:0]
				kv.NoValue = ArgsNoValue
			} else {
				kv.Value = DecodeArgAppend(kv.Value[:0], s.b[k:i])
			}

			s.b = s.b[i+1:]

			return true
		}
	}

	if isKey {
		kv.Key = DecodeArgAppend(kv.Key[:0], s.b)
		kv.Value = kv.Value[:0]
		kv.NoValue = ArgsNoValue
	} else {
		kv.Value = DecodeArgAppend(kv.Value[:0], s.b[k:])
	}

	s.b = s.b[len(s.b):]

	return true
}

// PeekScoped borrows the query/argument value associated with key into the given borrow scope.
func (a *Args) PeekScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := a.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}

	return borrow.NewBytes(b, nil)
}

// PeekAllScoped borrows all query/argument values associated with key into a slice of borrowed bytes.
func (a *Args) PeekAllScoped(s *borrow.Scope, key string) []borrow.Bytes {
	values := a.PeekMulti(key)
	if len(values) == 0 {
		return nil
	}

	res := make([]borrow.Bytes, len(values))
	for i, v := range values {
		res[i] = borrow.NewBytes(v, nil)
	}

	return res
}
