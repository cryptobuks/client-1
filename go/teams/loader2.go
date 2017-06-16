package teams

// loader2.go contains methods on TeamLoader.
// It would be normal for them to be in loader.go but hear me out.
// These functions do not call any functions in loader.go except for load2.
// They are here so that the files can be more manageable in size and
// people can work on loader.go and loader2.go simultaneously with less conflict.

// If links are needed in full that are stubbed in state, go out and get them from the server.
// Does not ask for any links above state's seqno, those will be fetched by getNewLinksFromServer.
func (l *TeamLoader) fillInStubbedLinks(state *keybase1.TeamData, needSeqnos []keybase1.Seqnos, proofSet *proofSetT) (*keybase1.TeamData, *proofSetT, error) {
	panic("TODO: implement")
}

func (l *TeamLoader) getNewLinksFromServer(teamID keybase1.TeamID, low keybase1.Seqno) (*teamUpdateT, error) {
	panic("TODO: implement")
}

// Verify that a link:
// - Was signed by a valid key for the user
// - Was signed by a user with permissions to make this link
// - Was signed
// But do not apply the link.
func (l *TeamLoader) verifyLink(state *keybase1.TeamData, link *SCChainLink, proofSet *proofSet) (*proofSet, error) {
	panic("TODO: implement")
}
