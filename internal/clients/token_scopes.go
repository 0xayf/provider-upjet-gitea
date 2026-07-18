// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clients

import (
	"sort"
	"strings"
)

// CanonicalTokenScopes returns the scope set in the form persisted by Gitea.
// A write scope includes read access to the same resource, so Gitea removes
// the redundant read scope when a token is created. Canonicalising both the
// request and observed state prevents that API behaviour from appearing as
// perpetual drift and rotating an otherwise-correct token on every poll.
func CanonicalTokenScopes(scopes []string) []string {
	set := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		set[scope] = struct{}{}
	}

	canonical := make([]string, 0, len(set))
	for scope := range set {
		access, resource, found := strings.Cut(scope, ":")
		if found && access == "read" {
			if _, writeExists := set["write:"+resource]; writeExists {
				continue
			}
		}
		canonical = append(canonical, scope)
	}
	sort.Strings(canonical)
	return canonical
}

// TokenScopesEqual compares two token scope sets using Gitea's canonical
// representation.
func TokenScopesEqual(a, b []string) bool {
	ac := CanonicalTokenScopes(a)
	bc := CanonicalTokenScopes(b)
	if len(ac) != len(bc) {
		return false
	}
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}
