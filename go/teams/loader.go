package teams

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/keybase/client/go/libkb"
	"github.com/keybase/client/go/protocol/keybase1"
)

type LoadTeamFreshness int

const (
	LoadTeamFreshnessRANCID LoadTeamFreshness = 0
	LoadTeamFreshnessAGED   LoadTeamFreshness = 1
	LoadTeamFreshnessFRESH  LoadTeamFreshness = 2
)

// Load a Team from the TeamLoader.
// Can be called from inside the teams package.
func Load(ctx context.Context, g *libkb.GlobalContext, lArg keybase1.LoadTeamArg) (*Team, error) {
	// teamData, err := g.GetTeamLoader().Load(ctx, lArg)
	// if err != nil {
	// 	return nil, err
	// }
	return nil, fmt.Errorf("TODO: implement team loader")
}

// Loader of keybase1.TeamData objects. Handles caching.
// Because there is one of this global object and it is attached to G,
// its Load interface must return a keybase1.TeamData not a teams.Team.
// To load a teams.Team use the package-level function Load.
// Threadsafe.
type TeamLoader struct {
	libkb.Contextified
	storage *Storage
	// Single-flight locks per team ID.
	locktab libkb.LockTable
}

func NewTeamLoader(g *libkb.GlobalContext, storage *Storage) *TeamLoader {
	return &TeamLoader{
		Contextified: libkb.NewContextified(g),
		storage:      storage,
	}
}

// NewTeamLoaderAndInstall creates a new loader and installs it into G.
func NewTeamLoaderAndInstall(g *libkb.GlobalContext) *TeamLoader {
	st := NewStorage(g)
	l := NewTeamLoader(g, st)
	g.SetTeamLoader(l)
	return l
}

func (l *TeamLoader) Load(ctx context.Context, lArg keybase1.LoadTeamArg) (res *keybase1.TeamData, err error) {
	me, err := l.getMe(ctx)
	if err != nil {
		return nil, err
	}
	return l.load1(ctx, me, lArg)
}

func (l *TeamLoader) getMe(ctx context.Context) (res keybase1.UserVersion, err error) {
	return loadUserVersionByUID(ctx, l.G(), l.G().Env.GetUID())
}

// Load1 unpacks the loadArg, calls load2, and does some final checks.
// The key difference between load1 and load2 is that load2 is recursive (for subteams).
func (l *TeamLoader) load1(ctx context.Context, me keybase1.UserVersion, lArg keybase1.LoadTeamArg) (*keybase1.TeamData, error) {
	err := l.checkArg(ctx, lArg)
	if err != nil {
		return nil, err
	}

	teamID := lArg.ID
	// Resolve the name to team ID. Will always hit the server for subteams.
	// It is safe for the answer to be wrong because the name is checked on the way out,
	// and the merkle tree check guarantees one sigchain per team id.
	if len(lArg.ID) == 0 {
		teamID, err = l.resolveNameToIDUntrusted(ctx, lArg.Name)
		if err != nil {
			return nil, err
		}
	}

	var res *keybase1.TeamData
	res, err = l.load2(ctx, load2ArgT{
		teamID: teamID,

		needAdmin:         lArg.NeedAdmin,
		needKeyGeneration: lArg.NeedKeyGeneration,
		wantMembers:       lArg.WantMembers,
		forceFullReload:   lArg.ForceFullReload,
		forceRepoll:       lArg.ForceRepoll,
		staleOK:           lArg.StaleOK,

		needSeqnos: nil,
		me:         me,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("team loader fault: got nil from load2")
	}

	// Sanity check the id
	if res != nil {
		if !res.Chain.Id.Eq(teamID) {
			return nil, fmt.Errorf("team id mismatch: %v != %v", res.Chain.Id.String(), teamID.String())
		}
	}

	// Check team name on the way out
	// It may have already been written to cache, but that should be ok.
	if len(lArg.Name) > 0 {
		if res != nil {
			if lArg.Name != res.Chain.Name {
				return nil, fmt.Errorf("team name mismatch: %v != %v", res.Chain.Name, lArg.Name)
			}
		}
	}

	return res, nil
}

func (l *TeamLoader) checkArg(ctx context.Context, lArg keybase1.LoadTeamArg) error {
	hasID := len(lArg.ID) > 0
	hasName := len(lArg.Name) > 0
	if !hasID && !hasName {
		return fmt.Errorf("team load arg must have either ID or Name")
	}
	return nil
}

// Resolve a team name to a team ID.
// Will always hit the server for subteams. The server can lie in this return value.
func (l *TeamLoader) resolveNameToIDUntrusted(ctx context.Context, teamName string) (keybase1.TeamID, error) {
	// TODO: Resolve the name to team ID.
	// For root team names, just hash.
	// For subteams, ask the server.
	panic("TODO: resolve team name to id")
}

// Mostly the same as the public keybase.LoadTeamArg
// but only supports loading by ID, and has neededSeqnos.
type load2ArgT struct {
	teamID keybase1.TeamID

	needAdmin         bool
	needKeyGeneration int
	wantMembers       []keybase1.UserVersion
	forceFullReload   bool
	forceRepoll       bool
	staleOK           bool

	needSeqnos []keybase1.Seqno

	me keybase1.UserVersion
}

// Load2 does the rest of the work loading a team.
// It is `playchain` described in the pseudocode in teamplayer.txt
func (l *TeamLoader) load2(ctx context.Context, arg load2ArgT) (*keybase1.TeamData, error) {
	var err error

	// Single-flight lock by team ID.
	lock := l.locktab.AcquireOnName(ctx, l.G(), arg.teamID.String())
	defer lock.Release(ctx)

	// Fetch from cache
	var ret *keybase1.TeamData
	if !arg.forceFullReload {
		// Load from cache
		ret = l.storage.Get(ctx, arg.teamID)
	}

	// Determine whether to repoll merkle.
	discardCache, repoll := l.load2DecideRepoll(ctx, arg, ret)
	if discardCache {
		ret = nil
		repoll = true
	}

	var lastSeqno keybase1.Seqno
	var lastLinkID keybase1.LinkID
	if (ret == nil) || repoll {
		// Reference the merkle tree to fetch the sigchain tail leaf for the team.
		lastSeqno, lastLinkID, err = l.lookupMerkle(ctx, arg.teamID)
	} else {
		lastSeqno = ret.Chain.LastSeqno
		lastLinkID = ret.Chain.LastLinkID
	}

	proofSet := newProofSet()
	var parentChildOperations []*parentChildOperation

	// Backfill stubbed links that need to be filled now.
	if ret != nil && len(arg.needSeqnos) > 0 {
		ret, proofSet, err = l.fillInStubbedLinks(ret, arg.needSeqnos, proofSet)
		if err != nil {
			return nil, err
		}
	}

	// Pull new links from the server
	var teamUpdate *teamUpdateT
	if ret == nil || ret.Chain.LastSeqno < lastSeqno {
		low := keybase1.Seqno(0)
		if ret != nil {
			low = ret.Chain.LastSeqno
		}
		teamUpdate, err = l.getNewLinksFromServer(arg.teamID, low)
		if err != nil {
			return nil, err
		}
	}

	links, err := teamUpdate.links()
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		if l.seqnosContains(arg.needSeqnos, link.Seqno) || arg.needAdmin {
			if link.isStubbed() {
				return nil, fmt.Errorf("team sigchain link %v stubbed when not allowed", link.Seqno)
			}
		}

		proofSet, err = l.verifyLink(ret, link, proofSet)
		if err != nil {
			return nil, err
		}

		if l.isParentChildOperation(&link) {
			parentChildOperations = append(parentChildOperations, l.toParentChildOperation(&link))
		}

		ret, err = l.applyNewLink(ret, link)
		if err != nil {
			return nil, err
		}
	}

	if !ret.Chain.LastLinkID.Eq(lastLinkID) {
		return nil, fmt.Errorf("wrong sigchain link ID: %v != %v",
			ret.Chain.LastLinkID, lastLinkID)
	}

	err = l.checkParentChildOperations(ret.Chain.ParentID, parentChildOperations)
	if err != nil {
		return nil, err
	}

	err = l.checkProofs(ret, proofSet)
	if err != nil {
		return nil, err
	}

	ret, err = l.addSecrets(ret, teamUpdate.Box, teamUpdate.Prevs, teamUpdate.ReaderKeyMasks)
	if err != nil {
		return nil, err
	}

	// Cache the validated result
	ret.CachedAt = NOW()
	l.storage.Put(ctx, ret)

	// Check request constraints
	err = l.load2CheckReturn(ctx, arg, ret)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

// Decide whether to repoll merkle based on load arg.
// Returns (discardCache, repoll)
// If discardCache is true, the caller should throw out their cached copy and repoll.
// Considers:
// - NeedAdmin
// - NeedKeyGeneration
// - WantMembers
// - ForceRepoll
// - Cache freshness / StaleOK
// - NeedSeqnos
func (l *TeamLoader) load2DecideRepoll(ctx context.Context, arg load2ArgT, fromCache *keybase1.TeamData) (bool, bool) {
	// NeedAdmin is a special constraint where we start from scratch.
	// Because of admin-only invite links.
	if arg.needAdmin {
		if !l.satisfiesNeedAdmin(ctx, arg.me, fromCache) {
			// Start from scratch if we are newly admin
			return true, true
		}
	}

	// Whether to hit up merkle for the latest tail.
	// This starts out false and then there are many reasons for turning it true.
	repoll := false

	if arg.forceRepoll {
		repoll = true
	}

	// Repoll to get a new key generation
	if arg.needKeyGeneration > 0 {
		if !l.satisfiesNeedKeyGeneration(ctx, arg.NeedKeyGeneration, fromCache) {
			repoll = true
		}
	}

	// Repoll because it might help get the wanted members
	if len(arg.wantMembers) > 0 {
		if !l.satisfiesWantMembers(ctx, arg.WantMembers, fromCache) {
			repoll = true
		}
	}

	// Repoll if we need a seqno not in the cache.
	// Does not force a repoll if we just need to fill in previous links
	if len(arg.needSeqnos) > 0 {
		if fromCache == nil {
			repoll = true
		} else {
			if fromCache.GetLatestSeqno() < l.seqnosMax(arg.NeedSeqnos) {
				repoll = true
			}
		}
	}

	if fromCache == nil {
		// We need a merkle leaf when starting from scratch.
		repoll = true
	}

	cacheIsOld := (fromCache != nil) && l.isFresh(fromCache.CachedAt)
	if cacheIsOld && !arg.StaleOK {
		// We need a merkle leaf
		repoll = true
	}

	return false, repoll
}

// Check whether the load produced a snapshot that can be returned to the caller.
// This should not check anything that is critical to validity of the snapshot
// because the snapshot is put into the cache before this check.
// Considers:
// - NeedAdmin
// - NeedKeyGeneration
func (l *TeamLoader) load2CheckReturn(ctx context.Context, arg load2ArgT, res *keybase1.TeamData) error {
	if arg.needAdmin {
		if !l.satisfiesNeedAdmin(ctx, arg.me, res) {
			l.G().Log.CDebugf(ctx, "user %v is not an admin of team %v at seqno:%v",
				arg.me, arg.ID, res.GetLatestSeqno())
			return fmt.Errorf("user is not an admin of the team", arg.me)
		}
	}

	// Repoll to get a new key generation
	if arg.needKeyGeneration > 0 {
		if !l.satisfiesNeedKeyGeneration(ctx, arg.NeedKeyGeneration, res) {
			return nil, fmt.Errorf("team key generation too low: %v < %v", foundGen, lArg.NeedKeyGeneration)
		}
	}
}

// Whether the user is an admin at the snapshot and there are no stubbed links.
func (l *TeamLoader) satisfiesNeedAdmin(ctx context.Context, me keybase1.UserVersion, teamData *keybase1.TeamData) bool {
	if teamData == nil {
		return false
	}
	role := TeamSigChainState{inner: res.Chain}.GetUserRole(me)
	if !(role == keybase1.TeamRole_OWNER || role == keybase1.TeamRole_ADMIN) {
		return false
	}
	if (TeamSigChainState{inner: teamData.Chain}.HasAnyStubbedLinks()) {
		return false
	}
	return true
}

func (l *TeamLoader) satisfiesNeedSeqnos(ctx context.Context, needSeqnos []keybase1.Seqno, teamData *keybase1.TeamData) error {
	if len(needSeqnos) == 0 {
		return nil
	}
	if teamData == nil {
		return fmt.Errorf("nil team does not contain needed seqnos")
	}
	panic("TODO: implement")
}

func (l *TeamLoader) lookupMerkle(ctx context.Context, teamID keybase1.TeamID) (keybase1.Seqno, keybase1.LinkID, error) {
	// TODO: make sure this punches through any caches.
	leaf, err := l.G().GetMerkleClient().LookupTeam(ctx, arg.TeamID)
	if err != nil {
		return nil, nil, err
	}
	if !leaf.TeamID.Eq(teamID) {
		return nil, nil, fmt.Errorf("merkle returned wrong leaf: %v != %v", leaf.TeamID.String(), teamID.String())
	}
	if leaf.Private == nil {
		return nil, nil, fmt.Errorf("merkle returned nil leaf")
	}
	return leaf.Private.Seqno, leaf.Private.LinkID, nil
}

// Whether y is in xs.
func (l *TeamLoader) seqnosContains(xs []keybase1.Seqno, y keybase1.Seqno) bool {
	for _, x := range xs {
		if x.Eq(y) {
			return true
		}
	}
	return false
}

func (l *TeamLoader) OnLogout() {
	l.storage.onLogout()
}
