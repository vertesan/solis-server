package uname

import "testing"

// TestCollectionsRegistry verifies every user collection is registered exactly
// once with the field used to identify its owning user.
func TestCollectionsRegistry(t *testing.T) {
	if len(Collections) != 73 {
		t.Fatalf("collection registry size = %d, want 73", len(Collections))
	}
	seen := make(map[string]struct{}, len(Collections))
	for _, collection := range Collections {
		if collection.Name == "" || collection.SUIDField == "" {
			t.Fatalf("invalid collection registry entry: %#v", collection)
		}
		if _, ok := seen[collection.Name]; ok {
			t.Fatalf("duplicate collection registry entry %q", collection.Name)
		}
		if collection.SUIDField != suidField &&
			!(collection.SUIDField == serverUserIDField && (collection.Name == UserAuth || collection.Name == UserPublic)) {
			t.Fatalf("unexpected deletion key for %q: %q", collection.Name, collection.SUIDField)
		}
		seen[collection.Name] = struct{}{}
	}
	if got := suidFieldFor(UserAuth); got != serverUserIDField {
		t.Fatalf("userAuth deletion field = %q, want %q", got, serverUserIDField)
	}
	if got := suidFieldFor(UserPublic); got != serverUserIDField {
		t.Fatalf("userPublic deletion field = %q, want %q", got, serverUserIDField)
	}
}

// suidFieldFor returns the expected ownership field for a user collection in
// the registry test.
func suidFieldFor(name string) string {
	for _, collection := range Collections {
		if collection.Name == name {
			return collection.SUIDField
		}
	}
	return ""
}
