package party

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func testDoc() (map[string]interface{}, string) {
	owner := primitive.NewObjectID()
	hex := owner.Hex()
	return map[string]interface{}{
		"ownerUserId":   owner,
		"memberUserIds": primitive.A{owner},
		"gameSetup": map[string]interface{}{
			"language": "Spanish",
			"playerCharacters": map[string]interface{}{
				hex: map[string]interface{}{
					"name": "Qararion Windsong", "level": 1.0, "class": "Explorer", "race": "Elfo",
				},
			},
		},
	}, hex
}

// The ready list comes back as primitive.A whenever a game document is decoded into a map, which is how every
// handler reads it. Asserting on []interface{} alone used to make this always false, so a fully ready party sat
// in the lobby forever because the client only starts the adventure when this reports true.
func TestAllMembersReadyReadsABSONArray(t *testing.T) {
	doc, hex := testDoc()
	cases := map[string]interface{}{
		"primitive.A":     primitive.A{hex},
		"[]interface{}":   []interface{}{hex},
		"[]string":        []string{hex},
		"primitive.A oid": primitive.A{primitive.ObjectID(primitive.NilObjectID)},
	}
	for name, readyRaw := range cases {
		party := map[string]interface{}{"readyUserIds": readyRaw}
		want := name != "primitive.A oid"
		if got := AllMembersReady(party, doc); got != want {
			t.Fatalf("%s: AllMembersReady = %v, want %v", name, got, want)
		}
	}
}

func TestAllMembersReadyIsFalseWhenSomeoneIsMissing(t *testing.T) {
	doc, _ := testDoc()
	if AllMembersReady(map[string]interface{}{"readyUserIds": primitive.A{}}, doc) {
		t.Fatal("nobody marked ready, so this must be false")
	}
	other := primitive.NewObjectID().Hex()
	if AllMembersReady(map[string]interface{}{"readyUserIds": primitive.A{other}}, doc) {
		t.Fatal("a stranger's ready flag must not count")
	}
	if AllMembersReady(map[string]interface{}{}, doc) {
		t.Fatal("a missing list must not count")
	}
}

func TestAllMembersReadyRequiresAMemberList(t *testing.T) {
	if AllMembersReady(map[string]interface{}{"readyUserIds": primitive.A{"a"}}, map[string]interface{}{}) {
		t.Fatal("a game with no members can never be ready")
	}
}

func TestCanonicalMemberIDsReadEitherArrayShape(t *testing.T) {
	owner := primitive.NewObjectID()
	for _, members := range []interface{}{
		primitive.A{owner},
		[]interface{}{owner},
	} {
		doc := map[string]interface{}{"ownerUserId": owner, "memberUserIds": members}
		got := CanonicalMemberIDStrings(doc)
		if len(got) != 1 || got[0] != owner.Hex() {
			t.Fatalf("expected the owner once, got %v for %T", got, members)
		}
	}
}
