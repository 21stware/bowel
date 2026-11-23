package main

import (
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// The exit lock as it really stands. It holds only where something brings
// the locked groups' traffic to the core: TUN, or the system proxy for the
// programs that follow it. The settings do not tell which of the two is
// there now, so the takeover's lock entry, the groups' rows and the
// issue are made from what was found.

// lockEntry is the takeover's word on the lock: nil when no group is
// locked or takeover carries it as set. guard is takeover off; fallback,
// takeover by TUN alone with TUN lost; held, that TUN or the system proxy
// brings traffic to the core now.
func lockEntry(locked, guard, fallback, held bool, proxyErr string) *Entry {
	if !locked {
		return nil
	}
	e := &Entry{Wanted: true, Status: "on"}
	switch {
	case guard && held:
	case fallback && held:
		e.Detail = i18n.T("TUN 没有生效，系统代理暂时指向 bowel 来守住锁定的路由组，只管得住跟随系统代理的程序。关掉再打开 TUN 会重试，TUN 恢复后系统代理会还原",
			"TUN isn't working, so the system proxy points at bowel for now to hold the locked groups; that only holds programs that follow the system proxy. Turning TUN off and on tries again, and the system proxy is put back once TUN works")
	case held:
		return nil
	case proxyErr != "": // bowel's own failure to take the system proxy
		e.Status, e.Detail = "error", proxyErr
	case fallback:
		e.Status, e.Detail = "error", i18n.T("TUN 没有生效，系统代理也没有指向 bowel：锁定的路由组现在没有守住，流量可能直连。关掉再打开 TUN 重试，或打开「系统代理」",
			"TUN isn't working and the system proxy doesn't point at bowel: the locked groups aren't held now and their traffic may go direct. Turn TUN off and on to retry, or turn the system proxy on")
	default:
		e.Status, e.Detail = "error", i18n.T("系统代理没有指向 bowel，锁定的路由组这时拦不住", "The system proxy doesn't point at bowel, so the locked groups aren't held now")
	}
	return e
}

// lockIssue is what takeover, being on, says of a lock that does not hold
// as set: an error when nothing holds it, a warning when the system proxy
// stands in for a TUN that is lost.
func lockIssue(l *Entry) *Issue {
	switch {
	case l == nil:
		return nil
	case l.Status == "error":
		return &Issue{Level: "error", Text: i18n.T("出口锁定现在没有守住", "The exit lock isn't holding now"), Detail: l.Detail}
	case l.Detail != "":
		return &Issue{Level: "warn", Text: i18n.T("TUN 没有生效，出口锁定暂由系统代理守着", "TUN isn't working; the system proxy holds the exit lock for now"), Detail: l.Detail}
	}
	return nil
}

// lockedOutSay is what a locked group with no node that works is told:
// the lock is what refuses its connections, and the ways out.
func lockedOutSay() string {
	return i18n.T("出口锁定正在拦下这个路由组的连接，不会改走直连。换一个能用的节点，或解除出口锁定。",
		"The exit lock is refusing this group's connections rather than sending them direct. Pick a node that works, or unlock.")
}

// unheldSay is what a locked group is told when nothing brings its
// traffic to bowel.
func unheldSay() string {
	return i18n.T("出口锁定现在没有守住：流量没有经过 bowel，这个路由组可能在直连", "The exit lock isn't holding now: traffic isn't going through bowel, so this group may be going direct")
}

// lockedNames are the locked groups that go by a node, for a sentence.
func lockedNames(all []RouteView) string {
	var names []string
	for _, r := range all {
		if r.Locked && !r.Direct {
			names = append(names, r.Name)
		}
	}
	return strings.Join(names, i18n.T("、", ", "))
}

// noneAlive says whether every node there is was tested on route rid and
// failed: a group that picks among all of them has no way out then, which
// matters once it is locked.
func noneAlive(nodes []NodeView, rid string) bool {
	for _, n := range nodes {
		if n = n.On(rid); !n.Tested || n.Delay > 0 {
			return false
		}
	}
	return len(nodes) > 0
}
