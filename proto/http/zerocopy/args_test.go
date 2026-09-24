// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/lemon4ksan/foundation/borrow"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

func TestArgsPool(t *testing.T) {
	a := zerocopy.AcquireArgs()
	if a == nil {
		t.Fatal("AcquireArgs returned nil")
	}
	a.Set("foo", "bar")
	if a.Len() != 1 {
		t.Fatalf("expected Len 1, got %d", a.Len())
	}
	zerocopy.ReleaseArgs(a)

	// Reacquire should return clean Args
	a2 := zerocopy.AcquireArgs()
	if a2.Len() != 0 {
		t.Fatalf("expected reacquired Args Len 0, got %d", a2.Len())
	}
	zerocopy.ReleaseArgs(a2)
}

func TestArgsQueryParsing(t *testing.T) {
	// Standard pairs
	a := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(a)

	a.Parse("a=1&b=2")
	if !bytes.Equal(a.Peek("a"), []byte("1")) || !bytes.Equal(a.Peek("b"), []byte("2")) {
		t.Fatalf("unexpected standard parse results: a=%s, b=%s", a.Peek("a"), a.Peek("b"))
	}

	// Multi-value keys
	a.Reset()
	a.Parse("foo=bar&foo=baz&foo=qux")
	multi := a.PeekMulti("foo")
	if len(multi) != 3 {
		t.Fatalf("expected 3 values for foo, got %d", len(multi))
	}
	if !bytes.Equal(multi[0], []byte("bar")) || !bytes.Equal(multi[1], []byte("baz")) ||
		!bytes.Equal(multi[2], []byte("qux")) {
		t.Fatalf("unexpected multi values: %v", multi)
	}

	// Flags without values
	a.Reset()
	a.Parse("flag1&flag2&normal=val")
	if !a.Has("flag1") || !a.Has("flag2") {
		t.Fatal("expected flag1 and flag2 to be present")
	}
	if len(a.Peek("flag1")) != 0 || len(a.Peek("flag2")) != 0 {
		t.Fatalf("expected empty value for flags, got %s and %s", a.Peek("flag1"), a.Peek("flag2"))
	}
	if !bytes.Equal(a.Peek("normal"), []byte("val")) {
		t.Fatalf("expected normal=val, got %s", a.Peek("normal"))
	}

	// Key with empty value
	a.Reset()
	a.Parse("emptyKey=")
	if !a.Has("emptyKey") {
		t.Fatal("expected emptyKey to be present")
	}
	if !bytes.Equal(a.Peek("emptyKey"), []byte{}) {
		t.Fatalf("expected empty byte slice for emptyKey, got %s", a.Peek("emptyKey"))
	}

	// URL-encoded keys and values
	a.Reset()
	a.Parse("a%20b=c%26d%3De")
	if !bytes.Equal(a.Peek("a b"), []byte("c&d=e")) {
		t.Fatalf("unexpected URL-decoded value: %s", a.Peek("a b"))
	}

	// Stray ampersands
	a.Reset()
	a.Parse("&&&a=1&&b=2&&")
	if a.Len() != 2 || !bytes.Equal(a.Peek("a"), []byte("1")) || !bytes.Equal(a.Peek("b"), []byte("2")) {
		t.Fatalf("unexpected parse with stray ampersands: len=%d", a.Len())
	}

	// Value containing equals
	a.Reset()
	a.Parse("query=a=1&b=2")
	if !bytes.Equal(a.Peek("query"), []byte("a=1")) {
		t.Fatalf("unexpected value with equals: %s", a.Peek("query"))
	}

	// Empty string
	a.Reset()
	a.Parse("")
	if a.Len() != 0 {
		t.Fatalf("expected 0 args for empty string, got %d", a.Len())
	}

	// ParseBytes
	a.Reset()
	a.ParseBytes([]byte("x=10&y=20"))
	if !bytes.Equal(a.Peek("x"), []byte("10")) || !bytes.Equal(a.Peek("y"), []byte("20")) {
		t.Fatalf("unexpected ParseBytes: x=%s, y=%s", a.Peek("x"), a.Peek("y"))
	}
}

func TestArgsCRUD(t *testing.T) {
	a := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(a)

	// Add variants
	a.Add("k1", "v1")
	a.AddBytesK([]byte("k2"), "v2")
	a.AddBytesV("k3", []byte("v3"))
	a.AddBytesKV([]byte("k4"), []byte("v4"))
	a.AddNoValue("k5")
	a.AddBytesKNoValue([]byte("k6"))

	if a.Len() != 6 {
		t.Fatalf("expected 6 args, got %d", a.Len())
	}
	if !bytes.Equal(a.Peek("k1"), []byte("v1")) ||
		!bytes.Equal(a.Peek("k2"), []byte("v2")) ||
		!bytes.Equal(a.Peek("k3"), []byte("v3")) ||
		!bytes.Equal(a.Peek("k4"), []byte("v4")) ||
		!a.Has("k5") || !a.Has("k6") {
		t.Fatal("Add variants failed")
	}

	// Set variants (overwriting existing)
	a.Set("k1", "v1_new")
	a.SetBytesK([]byte("k2"), "v2_new")
	a.SetBytesV("k3", []byte("v3_new"))
	a.SetBytesKV([]byte("k4"), []byte("v4_new"))
	a.SetNoValue("k1")
	a.SetBytesKNoValue([]byte("k2"))

	if !a.Has("k1") || !a.Has("k2") {
		t.Fatal("SetNoValue failed")
	}
	if !bytes.Equal(a.Peek("k3"), []byte("v3_new")) || !bytes.Equal(a.Peek("k4"), []byte("v4_new")) {
		t.Fatal("Set variants failed")
	}

	// Set non-existing
	a.Set("k7", "v7")
	if !bytes.Equal(a.Peek("k7"), []byte("v7")) {
		t.Fatal("Set non-existing failed")
	}

	// Del and DelBytes on *Args
	a.Del("k7")
	if a.Has("k7") {
		t.Fatal("Del failed")
	}
	a.DelBytes([]byte("k4"))
	if a.HasBytes([]byte("k4")) {
		t.Fatal("DelBytes failed")
	}

	// DelAllArgs and DelAllArgsStable package functions on []ArgsKV
	kvs := []zerocopy.ArgsKV{
		{Key: []byte("foo"), Value: []byte("1")},
		{Key: []byte("bar"), Value: []byte("2")},
		{Key: []byte("foo"), Value: []byte("3")},
		{Key: []byte("baz"), Value: []byte("4")},
		{Key: []byte("foo"), Value: []byte("5")},
	}
	kvsUnstable := zerocopy.DelAllArgs(slices.Clone(kvs), "foo")
	if len(kvsUnstable) != 2 {
		t.Fatalf("DelAllArgs failed: len=%d", len(kvsUnstable))
	}

	kvsStable := zerocopy.DelAllArgsStable(slices.Clone(kvs), "foo")
	if len(kvsStable) != 2 {
		t.Fatalf("DelAllArgsStable failed: len=%d", len(kvsStable))
	}
	if !bytes.Equal(kvsStable[0].Key, []byte("bar")) || !bytes.Equal(kvsStable[1].Key, []byte("baz")) {
		t.Fatalf("DelAllArgsStable altered order of remaining args")
	}

	// SetArgBytes, AppendArgBytes, PeekArgBytes
	argsSlice := make([]zerocopy.ArgsKV, 0, 4)
	argsSlice = zerocopy.SetArgBytes(argsSlice, []byte("alpha"), []byte("1"), zerocopy.ArgsHasValue)
	argsSlice = zerocopy.AppendArgBytes(argsSlice, []byte("alpha"), []byte("2"), zerocopy.ArgsHasValue)
	if len(argsSlice) != 2 {
		t.Fatalf("expected 2 args in slice, got %d", len(argsSlice))
	}
	peeked := zerocopy.PeekArgBytes(argsSlice, []byte("alpha"))
	if !bytes.Equal(peeked, []byte("1")) {
		t.Fatalf("PeekArgBytes failed: %s", peeked)
	}

	// CopyArgs
	dstSlice := zerocopy.CopyArgs(nil, argsSlice)
	if len(dstSlice) != 2 || !bytes.Equal(dstSlice[0].Key, []byte("alpha")) {
		t.Fatal("CopyArgs failed")
	}
}

func TestArgsTypeLookups(t *testing.T) {
	a := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(a)

	// Uint
	a.SetUint("u1", 42)
	a.SetUintBytes([]byte("u2"), 100)
	a.Set("u_invalid", "abc")

	u1, err := a.GetUint("u1")
	if err != nil || u1 != 42 {
		t.Fatalf("GetUint u1 failed: %d, %v", u1, err)
	}
	u2, err := a.GetUint("u2")
	if err != nil || u2 != 100 {
		t.Fatalf("GetUint u2 failed: %d, %v", u2, err)
	}
	if a.GetUintOrZero("u1") != 42 || a.GetUintOrZero("nonexistent") != 0 {
		t.Fatal("GetUintOrZero failed")
	}
	_, err = a.GetUint("u_invalid")
	if err == nil {
		t.Fatal("expected error on invalid uint")
	}
	_, err = a.GetUint("missing")
	if !errors.Is(err, zerocopy.ErrNoArgValue) {
		t.Fatalf("expected ErrNoArgValue, got %v", err)
	}

	// Ufloat
	a.Set("f1", "3.1415")
	a.Set("f_zero", "0")
	a.Set("f_neg", "-1.5")
	a.Set("f_invalid", "xyz")

	f1, err := a.GetUfloat("f1")
	if err != nil || f1 != 3.1415 {
		t.Fatalf("GetUfloat f1 failed: %f, %v", f1, err)
	}
	if a.GetUfloatOrZero("f1") != 3.1415 || a.GetUfloatOrZero("missing") != 0 {
		t.Fatal("GetUfloatOrZero failed")
	}
	_, err = a.GetUfloat("f_neg")
	if err == nil {
		t.Fatal("expected error for negative float")
	}
	_, err = a.GetUfloat("f_invalid")
	if err == nil {
		t.Fatal("expected error for invalid float")
	}
	_, err = a.GetUfloat("missing")
	if !errors.Is(err, zerocopy.ErrNoArgValue) {
		t.Fatalf("expected ErrNoArgValue, got %v", err)
	}

	// Bool
	trueValues := []string{"1", "t", "T", "true", "TRUE", "True", "y", "yes", "Y", "YES", "Yes"}
	for _, tv := range trueValues {
		a.Set("b", tv)
		if !a.GetBool("b") {
			t.Fatalf("expected GetBool=true for %q", tv)
		}
	}
	falseValues := []string{"0", "f", "false", "no", "other", "2", ""}
	for _, fv := range falseValues {
		a.Set("b", fv)
		if a.GetBool("b") {
			t.Fatalf("expected GetBool=false for %q", fv)
		}
	}
	if a.GetBool("missing_bool") {
		t.Fatal("expected false for missing bool")
	}
}

func TestArgsSortingAndSerialization(t *testing.T) {
	a := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(a)

	a.Add("banana", "2")
	a.Add("apple", "3")
	a.Add("apple", "1")
	a.Add("cherry", "4")

	// Sort (keys and values)
	a.Sort(bytes.Compare)
	var keys []string
	var vals []string
	for k, v := range a.All() {
		keys = append(keys, string(k))
		vals = append(vals, string(v))
	}
	expectedKeys := []string{"apple", "apple", "banana", "cherry"}
	expectedVals := []string{"1", "3", "2", "4"}
	if !slices.Equal(keys, expectedKeys) || !slices.Equal(vals, expectedVals) {
		t.Fatalf("Sort failed: keys=%v, vals=%v", keys, vals)
	}

	// SortKeys
	a.Reset()
	a.Add("zebra", "1")
	a.Add("ant", "2")
	a.SortKeys(bytes.Compare)
	if !bytes.Equal(a.Peek("ant"), []byte("2")) {
		t.Fatal("SortKeys failed")
	}

	// Serialization
	ser := a.AppendBytes(nil)
	if string(ser) != "ant=2&zebra=1" {
		t.Fatalf("AppendBytes failed: %s", ser)
	}
	qs := a.QueryString()
	if string(qs) != "ant=2&zebra=1" {
		t.Fatalf("QueryString failed: %s", qs)
	}
	str := a.String()
	if str != "ant=2&zebra=1" {
		t.Fatalf("String failed: %s", str)
	}

	var buf bytes.Buffer
	n, err := a.WriteTo(&buf)
	if err != nil || n != int64(len("ant=2&zebra=1")) || buf.String() != "ant=2&zebra=1" {
		t.Fatalf("WriteTo failed: %d, %v, %s", n, err, buf.String())
	}

	// Serialization with NoValue
	a.Reset()
	a.AddNoValue("flag")
	a.Add("key", "val")
	ser2 := a.AppendBytes(nil)
	if string(ser2) != "flag&key=val" {
		t.Fatalf("AppendBytes with NoValue failed: %s", ser2)
	}
}

func TestArgsBorrowing(t *testing.T) {
	a := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(a)

	a.Set("token", "secret42")
	a.Add("scope", "read")
	a.Add("scope", "write")

	s := borrow.NewScope()
	defer s.Release()

	// PeekScoped
	b := a.PeekScoped(s, "token")
	if !bytes.Equal(b.Bytes(), []byte("secret42")) {
		t.Fatalf("PeekScoped failed: %s", b.Bytes())
	}

	emptyB := a.PeekScoped(s, "nonexistent")
	if len(emptyB.Bytes()) != 0 {
		t.Fatalf("expected empty bytes for nonexistent, got %s", emptyB.Bytes())
	}

	// PeekAllScoped
	allB := a.PeekAllScoped(s, "scope")
	if len(allB) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(allB))
	}
	if !bytes.Equal(allB[0].Bytes(), []byte("read")) || !bytes.Equal(allB[1].Bytes(), []byte("write")) {
		t.Fatalf("unexpected PeekAllScoped: %v", allB)
	}

	emptyAll := a.PeekAllScoped(s, "nonexistent")
	if emptyAll != nil {
		t.Fatalf("expected nil for nonexistent, got %v", emptyAll)
	}
}

func TestArgsDecodeNoPlus(t *testing.T) {
	src := []byte("hello+world%20test%21")
	dst := zerocopy.DecodeArgAppendNoPlus(nil, src)
	// NoPlus should keep '+' as '+', while %20 -> space and %21 -> '!'
	expected := "hello+world test!"
	if string(dst) != expected {
		t.Fatalf("DecodeArgAppendNoPlus = %q, want %q", string(dst), expected)
	}

	// Test invalid % escapes in DecodeArgAppend and DecodeArgAppendNoPlus
	invalidHex := []byte("%2G%G2%2")
	dst2 := zerocopy.DecodeArgAppend(nil, invalidHex)
	if !bytes.Equal(dst2, invalidHex) {
		t.Fatalf("expected unescaped invalid hex to remain intact, got %s", dst2)
	}
	dst3 := zerocopy.DecodeArgAppendNoPlus(nil, invalidHex)
	if !bytes.Equal(dst3, invalidHex) {
		t.Fatalf("expected unescaped invalid hex to remain intact, got %s", dst3)
	}
}

func TestArgsAllEarlyBreak(t *testing.T) {
	a := zerocopy.AcquireArgs()
	defer zerocopy.ReleaseArgs(a)

	a.Add("k1", "v1")
	a.Add("k2", "v2")
	a.Add("k3", "v3")

	count := 0
	for range a.All() {
		count++
		if count == 2 {
			break
		}
	}
	if count != 2 {
		t.Fatalf("expected early break at count 2, got %d", count)
	}
}
