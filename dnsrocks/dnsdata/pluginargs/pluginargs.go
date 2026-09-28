/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package pluginargs owns the grammar of the argument list carried by a Plugin
// Map ("P") record, so that everything handling those arguments agrees on what
// is legal.
//
// The grammar is as follows:
//
//	args := pair ( ";" pair )*     ; the empty string means no arguments
//	pair := key "=" value
//	key  := one or more bytes, excluding ";" and "="
//	value := one or more bytes, excluding ";"
//
// Every argument carries a value; there is no bare-key form. A flag is written
// out, as in "debug=true". An empty pair is rejected wherever it appears, which
// rules out a leading or trailing ";" as well as ";;" — (SVCB tolerates a
// trailing delimiter and this does not).
//
// Duplicate keys are rejected. A reader is then free to collect the arguments
// into a map without silently discarding one of them.
//
// A value may contain "=" — only the first one separates key from value, as in
// SVCB, where base64 padding relies on it — but there is no escaping, so no key
// or value can contain ";". A value that needs a list of its own can use "|"
// between elements, as SVCB does, or whatever is most appropriate for the Plugin.
package pluginargs

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// argDelim separates one argument from the next.
	argDelim = ";"
	// kvSeparator separates a key from its value within an argument.
	kvSeparator = "="
)

var (
	// ErrEmptyArg is returned for an argument with no key or value at all,
	// which is what a leading, trailing or doubled delimiter produces.
	ErrEmptyArg = errors.New("empty argument")
	// ErrMissingSeparator is returned for an argument with no "=" at all. There
	// is no bare-key form; write the value out.
	ErrMissingSeparator = errors.New("argument has no value")
	// ErrEmptyKey is returned for an argument whose key is missing.
	ErrEmptyKey = errors.New("argument has no key")
	// ErrEmptyValue is returned for an argument whose value is missing.
	ErrEmptyValue = errors.New("argument has a separator but no value")
	// ErrDuplicateKey is returned when a key appears more than once.
	ErrDuplicateKey = errors.New("duplicate argument key")
)

// Get returns the value of key in args, and whether the key is present.
// It assumes args has already passed Validate.
func Get(args, key string) (value string, found bool) {
	for arg := range strings.SplitSeq(args, argDelim) {
		if k, v, ok := strings.Cut(arg, kvSeparator); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// Validate reports whether args is a legal Plugin Map argument list. The empty
// string is legal and means no arguments.
func Validate(args string) error {
	if args == "" {
		return nil
	}

	seen := make(map[string]struct{})
	for arg := range strings.SplitSeq(args, argDelim) {
		if arg == "" {
			return ErrEmptyArg
		}
		key, value, hasSeparator := strings.Cut(arg, kvSeparator)
		if !hasSeparator {
			return fmt.Errorf("%q: %w", arg, ErrMissingSeparator)
		}
		if key == "" {
			return fmt.Errorf("%q: %w", arg, ErrEmptyKey)
		}
		if value == "" {
			return fmt.Errorf("%q: %w", arg, ErrEmptyValue)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%q: %w", key, ErrDuplicateKey)
		}
		seen[key] = struct{}{}
	}
	return nil
}
