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
		teamID, err = l.resolveNameToID(ctx, lArg.Name)
		if err != nil {
			return nil, err
		}
	}

	res, err := l.load2()
	if err != nil {
		return nil, err
	}

	var res *keybase1.TeamData
	res, err = l.load2(ctx, load2ArgT{
		teamID:      teamID,
		needAdmin:   lArg.NeedAdmin,
		forceRepoll: lArg.ForceRepoll,
		needSeqnos:  nil,
		fromCache:   fromCache,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		// TODO: decide whether load2 can return (nil, err:nil) or not
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
func (l *TeamLoader) resolveNameToID(ctx context.Context, teamName string) (keybase1.TeamID, error) {
	// TODO: Resolve the name to team ID.
	// For root team names, just hash.
	// For subteams, ask the server.
	panic("TODO: resolve team name to id")
}

type load2ArgT struct {
	teamID      keybase1.TeamID
	needAdmin   bool
	forceRepoll bool
	needSeqnos  []keybase1.Seqno

	me        keybase1.UserVersion
	fromCache *keybase1.TeamData // nil when loading from scratch
}

// Load2 does the rest of the work loading a team.
// It is `playchain` described in the pseudocode in teamplayer.txt
func (l *TeamLoader) load2(ctx context.Context, arg load2ArgT) (*keybase1.TeamData, error) {
	// Single-flight lock by team ID.
	lock := l.locktab.AcquireOnName(ctx, l.G(), arg.teamID.String())
	defer lock.Release(ctx)

	// Fetch from cache
	var ret *keybase1.TeamData
	if !lArg.ForceFullReload {
		// Load from cache
		fromCache = l.storage.Get(ctx, teamID)
	}

	// NeedAdmin is a special constraint where we start from scratch.
	// Because of admin-only invite links.
	if arg.NeedAdmin {
		if !l.satisfiesNeedAdmin(ctx, arg.me, fromCache) {
			// Start from scratch if we are newly admin
			ret = nil
		}
	}

	// Whether to hit up merkle for the latest tail.
	// This starts out false and then there are many reasons for turning it true.
	repoll := false

	if arg.NeedKeyGeneration > 0 {
		if !l.satisfiesNeedKeyGeneration(ctx, arg.NeedKeyGeneration, fromCache) {
			repoll = true
		}
	}

	if len(arg.WantMembers) > 0 {
		if !l.satisfiesWantMembers(ctx, arg.WantMembers, fromCache) {
			repoll = true
		}
	}

	if len(arg.NeedSeqnos) > 0 {
		if fromCache == nil {
			repoll = true
		} else {
			if fromCache.GetLatestSeqno() < l.seqnosMax(arg.NeedSeqnos) {
				repoll = true
			}
		}
	}

	if ret == nil {
		// We need a merkle leaf when starting from scratch.
		repoll = true
	}

	cacheIsOld := (ret != nil) && l.isFresh(ret.CachedAt)
	if cacheIsOld && !arg.StaleOK && cacheTooOld {
		// We need a merkle leaf
		repoll = true
	}

	// Check the requested constraints from arg against a snapshot.
	// Used to decide whether to ask for an update even if the cache is fresh.
	checkConstraints := func(teamData *keybase1.TeamData) error {
		if arg.NeedAdmin {
			if !l.satisfiesNeedAdmin(ctx, arg.me, teamData) {
				// Start from scratch if we are newly admin
				ret = nil
			}
		}
		panic("TODO: check other constraints")
	}

	// TODO: wait: it doesn't make sense to do this on the way out because of the lack of return guarantee in needMember.
	err = checkConstraints(ret)
	failedRequestConstraints = err != nil
	if err != nil {
		s.G().Log.Debug("teams.Loader punching through due to failed constraint: %v", err)
	}
	err = nil

	panic("TODO: check load constraints, maybe")

	cacheTooOld := (ret != nil) && l.isFresh(ret.CachedAt)
	var lastSeqno keybase1.Seqno
	var lastSigID keybase1.SigID
	if arg.forceRepoll || (ret == nil) || cacheTooOld || failedRequestConstraints {
		// Reference the merkle tree to fetch the sigchain tail leaf for the team.
		lastSeqno, lastSigID, err = l.lookupMerkle(ctx, arg.TeamID)
	} else {
		lastSeqno = ret.LastSeqno()
		lastSigID = ret.LastSigID()
	}

	// Check admin on the way out.
	if arg.NeedAdmin {
		if !l.satisfiesNeedAdmin(ctx, arg.me, ret) {
			return nil, fmt.Errorf("user is not an admin of the team")
		}
	}

	// Check key generation on the way out
	if lArg.NeedKeyGeneration != 0 {
		if res != nil {
			foundGen := len(res.PerTeamKeySeeds)
			if foundGen < lArg.NeedKeyGeneration {
				return nil, fmt.Errorf("team key generation too low: %v < %v", foundGen, lArg.NeedKeyGeneration)
			}
		}
	}

	// Cache the validated result
	l.storage.Put(ctx, res)

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
	return nil
}

func (l *TeamLoader) satisfiesNeedSeqnos(ctx, context.Context, needSeqnos []keybase1.Seqno,  teamData *keybase1.TeamData) error {
	if len(needSeqnos) == 0 {
		return nil
	}
	if teamData == nil {
		return fmt.Errorf("nil team does not contain needed seqnos")
	}
}

func (l *TeamLoader) lookupMerkle(ctx context.Context, teamID keybase1.TeamID) (keybase1.Seqno, keybase1.SigID, error) {
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

func (l *TeamLoader) OnLogout() {
	l.storage.onLogout()
}
