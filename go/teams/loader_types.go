package teams

// Collection of ordering constraints waiting to be verified.
// TODO implement
type proofSetT struct{}

func newProofSet() *proofSetT {
	&proofSetT{}
}

// --------------------------------------------------

// An operation
// TODO implement
type parentChildOperation struct {
}

// --------------------------------------------------

// A server response containing new links
// as well as readerKeyMasks and per-team-keys.
// TODO implement (may be exactly rawTeam)
type teamUpdateT struct {
}

func (t *teamUpdateT) links() ([]SCChainLink, error) {
	panic("TODO: implement")
}
