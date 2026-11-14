package ruleset

// A whole Clash config where a rule list was expected. It is told apart, so
// that it gets an answer of its own instead of "no usable rules", and the
// rules it sends to one target can be taken out of it: without the target,
// which is whoever imports them's to choose, and without the config's
// groups, nodes, rule-providers or DNS.

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/i18n"
)

// ConfigTarget is one target of a whole config, and how many of the rules
// sent to it can be imported.
type ConfigTarget struct {
	Name    string
	Count   int
	Skipped int // rules to it that can't be: MATCH, RULE-SET, what the core's lists don't take
}

// ConfigError says a source is a whole config, not a rule list. Targets are
// those with rules that can be imported, as the config first names them.
type ConfigError struct {
	Targets []ConfigTarget
	Skipped int // every rule of the config that can't be imported
}

func (e *ConfigError) Error() string {
	if len(e.Targets) == 0 {
		return i18n.T("这是一份完整配置，里面没有可以单独导入的规则", "This is a whole config, and none of its rules can be imported on their own")
	}
	return i18n.T("这是一份完整配置，不是规则集", "This is a whole config, not a rule list")
}

// targetMark ends a source that takes one target's rules out of a config.
// It reads as a link's fragment, which is never sent to the server.
const targetMark = "#rules="

// Target splits a source into where it is and the target it names, "" for a
// plain list.
func Target(source string) (location, target string) {
	if i := strings.LastIndex(source, targetMark); i >= 0 {
		return source[:i], source[i+len(targetMark):]
	}
	return source, ""
}

// WithTarget is the source for target's rules of the config at location.
func WithTarget(location, target string) string {
	if target == "" {
		return location
	}
	return location + targetMark + target
}

// config is what a whole config sends where: each target's rules in the form
// the core's lists take, in the config's order.
type config struct {
	names   []string // targets, as first named
	rules   map[string][]string
	skipped map[string]int
}

// readConfig reads data as a whole config; false for anything else. A
// rule-provider has a payload and a text list is no YAML mapping, so the
// keys only a config has settle it.
func readConfig(data []byte) (*config, bool) {
	var y map[string]any
	if yaml.Unmarshal(data, &y) != nil {
		return nil, false
	}
	if _, list := y["payload"]; list {
		return nil, false
	}
	if !slices.ContainsFunc([]string{"rules", "proxies", "proxy-groups", "proxy-providers", "rule-providers"}, func(k string) bool { _, ok := y[k]; return ok }) {
		return nil, false
	}
	c := &config{rules: map[string][]string{}, skipped: map[string]int{}}
	lines, _ := y["rules"].([]any)
	for _, v := range lines {
		line, _ := v.(string)
		bare, target := splitRule(line)
		if !slices.Contains(c.names, target) {
			c.names = append(c.names, target)
		}
		if r, ok := classicalRule(bare); ok && target != "" {
			c.rules[target] = append(c.rules[target], r)
		} else {
			c.skipped[target]++
		}
	}
	return c, true
}

// splitRule takes the target out of a config's rule, the way the core finds
// it (rules/common ParseRulePayload): the third field, or the last one of a
// type whose payload holds commas.
func splitRule(line string) (bare, target string) {
	item := strings.Split(line, ",")
	for i := range item {
		item[i] = strings.TrimSpace(item[i])
	}
	kind := strings.ToUpper(item[0])
	switch {
	case len(item) < 2:
		return line, ""
	case kind == "MATCH":
		return kind, item[1]
	case commaPayload[kind] || kind == "SUB-RULE":
		return strings.Join(item[:len(item)-1], ","), item[len(item)-1]
	case len(item) < 3:
		return line, ""
	}
	return strings.Join(slices.Concat(item[:2], item[3:]), ","), item[2]
}

func (c *config) err() *ConfigError {
	e := &ConfigError{}
	for _, name := range c.names {
		e.Skipped += c.skipped[name]
		if n := len(c.rules[name]); n > 0 {
			e.Targets = append(e.Targets, ConfigTarget{Name: name, Count: n, Skipped: c.skipped[name]})
		}
	}
	return e
}

// parseTarget reads the rules a whole config sends to target.
func parseTarget(data []byte, target string) (Set, error) {
	c, ok := readConfig(data)
	if !ok {
		return Set{}, errors.New(i18n.T("这个来源不是完整配置，没有目标可选", "This source isn't a whole config, so it has no targets to take rules from"))
	}
	if len(c.rules[target]) == 0 {
		return Set{}, fmt.Errorf(i18n.T("配置里没有发往「%s」的可用规则", "The config has no usable rules for %s"), target)
	}
	return Set{Behavior: Classical, Rules: c.rules[target], Skipped: c.skipped[target]}, nil
}
