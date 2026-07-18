// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clients

import (
	"reflect"
	"testing"
)

func TestCanonicalTokenScopes(t *testing.T) {
	got := CanonicalTokenScopes([]string{
		"write:package",
		"read:user",
		"read:package",
		"write:package",
	})
	want := []string{"read:user", "write:package"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CanonicalTokenScopes() = %v, want %v", got, want)
	}
}

func TestTokenScopesEqualTreatsWriteAsIncludingRead(t *testing.T) {
	if !TokenScopesEqual(
		[]string{"read:package", "write:package", "read:user"},
		[]string{"read:user", "write:package"},
	) {
		t.Fatal("expected canonical scope sets to be equal")
	}
}

func TestTokenScopesEqualDetectsRealScopeDrift(t *testing.T) {
	if TokenScopesEqual(
		[]string{"write:package", "read:user"},
		[]string{"write:package"},
	) {
		t.Fatal("expected distinct scope sets to remain different")
	}
}
