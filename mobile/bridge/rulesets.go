package bridge

// Rule sets on the phone, as the desktop has them in app_rulesets.go: a
// service whose rules come from a link or a file. The list is fetched and
// checked here, shown before it counts, and handed to the core as a file of
// its own.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/ruleset"
	"github.com/oboo/moho/internal/settings"
)

// retryAfter is how long a set whose download failed waits for the next try.
const retryAfter = time.Hour

// ruleSetData is a rule set as its row shows it.
type ruleSetData struct {
	Where     string     `json:"where"` // a link's host, or the file's name
	Local     bool       `json:"local"`
	Target    string     `json:"target,omitempty"` // of a whole config
	Have      bool       `json:"have"`             // false when the saved copy is gone
	Count     int        `json:"count"`
	Skipped   int        `json:"skipped"`
	UpdatedAt int64      `json:"updatedAt"`
	Error     string     `json:"error,omitempty"` // of the last update; the rules are the last good ones
	Preview   []ruleData `json:"preview"`
}

type targetData struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Skipped int    `json:"skipped"`
}

type dupData struct {
	Group     string `json:"group"`
	GroupName string `json:"groupName"`
	Service   string `json:"service"`
}

// ruleSetCheck is a source looked at, before anything is saved: a list, the
// set it is already imported as, or a whole config's targets to choose from.
type ruleSetCheck struct {
	Count   int          `json:"count"`
	Skipped int          `json:"skipped"`
	Where   string       `json:"where"`
	Name    string       `json:"name"`
	Target  string       `json:"target,omitempty"`
	Rules   []ruleData   `json:"rules"`
	Dup     *dupData     `json:"dup,omitempty"`
	Targets []targetData `json:"targets,omitempty"`
	Left    int          `json:"left,omitempty"` // rules of a whole config that can't be imported
}

// openRuleSets opens the sets' folder, inside the core's own so that the
// core may read it, and clears the files no service names any more.
func (x *bridge) openRuleSets() error {
	store, err := ruleset.Open(filepath.Join(x.dir, "rulesets"))
	if err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, sv := range x.set.Get().Routing.Services {
		if sv.Source != "" {
			ids[sv.ID] = true
		}
	}
	store.Keep(ids)
	x.ruleSets = store
	return nil
}

// ruleSetRefs are the sets the core should load.
func (x *bridge) ruleSetRefs(s settings.Settings) []core.RuleSetRef {
	var out []core.RuleSetRef
	for _, sv := range s.Routing.Services {
		if sv.Source == "" || sv.Off {
			continue
		}
		if m, ok := x.ruleSets.Get(sv.ID); ok {
			out = append(out, core.RuleSetRef{ID: sv.ID, Path: x.ruleSets.Path(sv.ID), Behavior: m.Behavior})
		}
	}
	return out
}

func rulesData(list []routes.Rule) []ruleData {
	out := []ruleData{}
	for _, l := range list {
		out = append(out, ruleData{Text: l.Text, Kind: l.Kind, KindName: routes.KindName(l.Kind), Value: l.Value, Shown: l.Shown()})
	}
	return out
}

// ruleSetWhere is a source in a word or two: a link's host, a file's name.
func ruleSetWhere(source string) string {
	location, _ := ruleset.Target(source)
	if u, err := url.Parse(location); err == nil && !ruleset.Local(location) {
		return u.Host
	}
	return location
}

func (x *bridge) ruleSetView(sv routes.Service) *ruleSetData {
	_, target := ruleset.Target(sv.Source)
	d := &ruleSetData{Where: ruleSetWhere(sv.Source), Local: ruleset.Local(sv.Source), Target: target, Preview: []ruleData{}}
	if m, ok := x.ruleSets.Get(sv.ID); ok {
		d.Have, d.Count, d.Skipped, d.UpdatedAt, d.Error = true, m.Count, m.Skipped, m.UpdatedAt.UnixMilli(), m.Error
		d.Preview = rulesData(x.ruleSets.Preview(sv.ID))
	}
	return d
}

// ruleSetSource is what the page asks about: a link as typed, or a file that
// was picked, and for a whole config the target chosen. A picked file is a
// copy that goes with the next pick, so the set keeps its name, not its path.
func ruleSetSource(a callArgs) (fetch, keep string, err error) {
	if a.Path != "" {
		return ruleset.WithTarget(a.Path, a.Text), ruleset.WithTarget(filepath.Base(a.Path), a.Text), nil
	}
	link := strings.TrimSpace(a.Input)
	if ruleset.Local(link) {
		return "", "", errors.New(i18n.T("请粘贴 http 或 https 链接", "Paste an http or https link"))
	}
	link = ruleset.WithTarget(link, a.Text)
	return link, link, nil
}

// fetchRuleSet gets and reads a source. Nothing is saved, and nothing in the
// settings changes: a source that fails is only an error to show.
func (x *bridge) fetchRuleSet(ctx context.Context, a callArgs) (*ruleset.Checked, error) {
	fetch, keep, err := ruleSetSource(a)
	if err != nil {
		return nil, err
	}
	c, err := ruleset.Fetch(ctx, x.httpClient(), fetch)
	if err != nil {
		return nil, err
	}
	c.Source = keep
	return c, nil
}

func (x *bridge) routeTitle(id string) string {
	if r, err := x.route(id); err == nil {
		return routes.Title(r)
	}
	return id
}

func (x *bridge) checkRuleSet(ctx context.Context, a callArgs) (ruleSetCheck, error) {
	x.mu.Lock()
	x.checked = nil
	x.mu.Unlock()
	fetch, keep, err := ruleSetSource(a)
	if err != nil {
		return ruleSetCheck{}, err
	}
	if sv, dup := routes.FindSource(x.routes(), keep); dup && a.Path == "" {
		return ruleSetCheck{Rules: []ruleData{}, Dup: &dupData{Group: sv.Group, GroupName: x.routeTitle(sv.Group), Service: sv.ID}}, nil
	}
	c, err := x.fetchRuleSet(ctx, a)
	var cfg *ruleset.ConfigError
	if errors.As(err, &cfg) && len(cfg.Targets) > 0 {
		out := ruleSetCheck{Where: ruleSetWhere(keep), Rules: []ruleData{}, Left: cfg.Skipped}
		for _, t := range cfg.Targets {
			out.Targets = append(out.Targets, targetData{Name: t.Name, Count: t.Count, Skipped: t.Skipped})
		}
		return out, nil
	}
	if err != nil {
		return ruleSetCheck{}, err
	}
	// What was shown is what 导入 saves: the picked file may be gone by then,
	// and a link is not downloaded twice.
	x.mu.Lock()
	x.checked, x.checkedFrom = c, fetch
	x.mu.Unlock()
	return ruleSetCheck{Count: len(c.Set.Rules), Skipped: c.Set.Skipped, Where: ruleSetWhere(c.Source), Name: c.Name(), Target: a.Text, Rules: rulesData(c.Preview())}, nil
}

// importRuleSet makes a source a service of group a.Route, and closes the
// open connections it now sends there, so that it shows at once. It returns
// the service's id.
func (x *bridge) importRuleSet(ctx context.Context, a callArgs) (string, error) {
	if _, err := x.route(a.Route); err != nil {
		return "", err
	}
	fetch, _, err := ruleSetSource(a)
	if err != nil {
		return "", err
	}
	x.mu.Lock()
	c, from := x.checked, x.checkedFrom
	x.checked = nil
	x.mu.Unlock()
	if c == nil || from != fetch {
		if c, err = x.fetchRuleSet(ctx, a); err != nil {
			return "", err
		}
	}
	if sv, dup := routes.FindSource(x.routes(), c.Source); dup && a.Path == "" {
		return "", fmt.Errorf(i18n.T("已经导入到「%s」了", "Already imported to %s"), x.routeTitle(sv.Group))
	}
	var id string
	var saveErr error
	err = x.change(ctx, func(s *settings.Settings) {
		id = s.Routing.AddRuleSet(s.Rules, a.Route, c.Name(), c.Source)
		// Before the config is written: it may only name a file that's there.
		if saveErr = x.ruleSets.Save(id, c); saveErr != nil {
			s.Rules = s.Routing.RemoveService(s.Rules, id)
		}
	})
	if err = errors.Join(err, saveErr); err != nil {
		if id != "" {
			x.ruleSets.Delete(id)
		}
		return "", err
	}
	x.rerouteBy(ctx, func(host, ip string) bool { _, ok := x.ruleSets.Match(id, host, ip); return ok }, a.Route)
	return id, nil
}

// updateRuleSet fetches a set again. A failure leaves the last good list in
// use and shows on its row. The core reads the one file again, which closes
// no connection.
func (x *bridge) updateRuleSet(ctx context.Context, id string) error {
	defer x.push()
	var conf *routes.ServiceConf
	s := x.set.Get()
	for i := range s.Routing.Services {
		if sv := &s.Routing.Services[i]; sv.ID == id && sv.Source != "" {
			conf = sv
		}
	}
	if conf == nil {
		return errors.New(i18n.T("没有这个规则集", "No such rule set"))
	}
	if ruleset.Local(conf.Source) {
		return errors.New(i18n.T("这个规则集来自文件，要换内容请重新导入", "This rule set came from a file. Import it again to change it"))
	}
	x.mu.Lock()
	if x.setTried == nil {
		x.setTried = map[string]time.Time{}
	}
	x.setTried[id] = time.Now()
	x.mu.Unlock()
	before, had := x.ruleSets.Get(id)
	meta, files := x.ruleSets.Snapshot(map[string]bool{id: true})
	if err := x.ruleSets.Update(ctx, x.httpClient(), id, conf.Source); err != nil {
		return err
	}
	if conf.Off {
		return nil
	}
	// The config names the file and what kind of list it is: a set that
	// wasn't there, or changed kind, needs a new one.
	var err error
	if after, _ := x.ruleSets.Get(id); !had || after.Behavior != before.Behavior {
		err = x.applyConfig(ctx)
	} else if c, _ := x.core(); c != nil {
		err = c.ReloadRuleSet(ctx, routes.RuleSetProvider(id))
	}
	if err != nil {
		old := meta[id]
		old.Error = err.Error()
		x.setMu.Lock()
		restoreErr := x.ruleSets.Restore(id, old, files[id])
		writeErr := x.writeConfig()
		x.setMu.Unlock()
		return errors.Join(err, restoreErr, writeErr)
	}
	return nil
}

// updateRuleSets downloads the sets from links that are due at now. One
// whose download failed is tried again after a while, not at every look.
func (x *bridge) updateRuleSets(now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, sv := range x.set.Get().Routing.Services {
		if sv.Source == "" || sv.Off || ruleset.Local(sv.Source) || !x.ruleSets.Stale(sv.ID, now) {
			continue
		}
		x.mu.Lock()
		tried, again := x.setTried[sv.ID]
		x.mu.Unlock()
		if again && now.Sub(tried) < retryAfter {
			continue
		}
		_ = x.updateRuleSet(ctx, sv.ID)
	}
}

// removeService deletes a service, and with a rule set its file.
func (x *bridge) removeService(ctx context.Context, id string) error {
	if err := x.change(ctx, func(s *settings.Settings) { s.Rules = s.Routing.RemoveService(s.Rules, id) }); err != nil {
		return err
	}
	x.ruleSets.Delete(id)
	return nil
}
