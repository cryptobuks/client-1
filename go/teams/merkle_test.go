package teams

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/keybase/client/go/kbtest"
	"github.com/stretchr/testify/require"
)

// Test getting the merkle leaf from the server.
// This is a test of MerkleClient.
func TestMerkle(t *testing.T) {
	tc := SetupTest(t, "team", 1)
	defer tc.Cleanup()

	kbtest.CreateAndSignupFakeUser("team", tc.G)

	name := createTeam(tc)

	team, err := Get(context.TODO(), tc.G, name)
	require.NoError(t, err)

	leaf, err := tc.G.MerkleClient.LookupTeam(context.TODO(), team.Chain.GetID())
	require.NoError(t, err)
	require.NotNil(t, leaf)
	t.Logf("team merkle leaf: %v", spew.Sdump(leaf))
	if leaf.TeamID.IsNil() {
		t.Fatalf("nil teamID; likely merkle hasn't yet published and polling is busted")
	}
	require.Equal(t, team.Chain.GetID(), leaf.TeamID, "team id")
	require.Equal(t, team.Chain.GetLatestSeqno(), leaf.Private.Seqno)
	require.Equal(t, team.Chain.GetLatestLinkID(), leaf.Private.LinkID.Export())
	// leaf.Private.SigID not checked
	require.Nil(t, leaf.Public, "team public leaf")
}
