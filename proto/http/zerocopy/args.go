// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"github.com/lemon4ksan/mach/proto/semantic/uri"
)

// Args represents HTTP query parameters or form arguments (RFC 3986 §3.4).
type Args = uri.Args

// ArgsKV represents an individual key-value query argument pair.
type ArgsKV = uri.ArgsKV

const (
	// ArgsNoValue specifies that an argument has no value component.
	ArgsNoValue = uri.ArgsNoValue
	// ArgsHasValue specifies that an argument contains a value component.
	ArgsHasValue = uri.ArgsHasValue
)

var (
	// ErrNoArgValue is returned when an argument has no value.
	ErrNoArgValue = uri.ErrNoArgValue

	// AcquireArgs returns an empty [Args] instance from the pool.
	AcquireArgs = uri.AcquireArgs

	// ReleaseArgs resets a and returns it to the pool.
	ReleaseArgs = uri.ReleaseArgs

	// CopyArgs copies src args to dst.
	CopyArgs = uri.CopyArgs

	// DelAllArgsStable deletes all arguments with key while preserving order.
	DelAllArgsStable = uri.DelAllArgsStable

	// DelAllArgs deletes all arguments with key.
	DelAllArgs = uri.DelAllArgs

	// SetArgBytes sets key-value argument.
	SetArgBytes = uri.SetArgBytes

	// SetArg sets key-value argument.
	SetArg = uri.SetArg

	// AppendArgBytes appends key-value argument.
	AppendArgBytes = uri.AppendArgBytes

	// AppendArg appends key-value argument.
	AppendArg = uri.AppendArg

	// AllocArg allocates an argument pair.
	AllocArg = uri.AllocArg

	// ReleaseArg releases an argument pair.
	ReleaseArg = uri.ReleaseArg

	// PeekArgBytes peeks argument value bytes by key bytes.
	PeekArgBytes = uri.PeekArgBytes

	// PeekArgStr peeks argument value bytes by key string.
	PeekArgStr = uri.PeekArgStr
)
