package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oboo/moho/internal/i18n"
)

// bowel bar: a line for a status bar (waybar, polybar, i3blocks and the
// like) every second, with the speeds now, and what is behind them in the
// tooltip. It goes on until the bar stops reading, and waits through bowel
// not running or restarting, so the bar's module is started once.
//
// waybar reads a JSON object a line (a custom module with "return-type":
// "json"): text and tooltip in Pango markup, and classes to style by. The
// other bars read plain text (--format text). --once prints one line and
// ends, for bars that run the command on an interval.

var barFormats = []string{"waybar", "text"}

// barEvery is how often the bar asks, unless a test set its own Poll.
const barEvery = time.Second

func runBar(c *Ctx) error {
	format := c.flag("format")
	if format == "" {
		format = "waybar"
	}
	every := barEvery
	if c.env.Poll != defaultPoll {
		every = c.env.Poll
	}
	for {
		var st status
		_, err := c.get("get_status", nil, &st)
		if err != nil && c.cl != nil {
			// The connection is from before a restart: the next ask dials again.
			c.cl.close()
			c.cl = nil
		}
		line := barLine(st, err == nil, format)
		if _, werr := fmt.Fprintln(c.out, line); werr != nil {
			return nil // the bar has stopped reading
		}
		if c.has("once") {
			return nil
		}
		select {
		case <-c.env.Context.Done():
			return nil
		case <-time.After(every):
		}
	}
}

// barClass is the takeover in a word: on, off, paused, or stopped when
// bowel isn't running.
func barClass(st status, running bool) string {
	tk := st.Takeover
	switch {
	case !running:
		return "stopped"
	case tk.On:
		return "on"
	case tk.System.Status == "paused" || tk.Tun.Status == "paused":
		return "paused"
	}
	return "off"
}

// barSpeeds is the speeds now, down then up.
func barSpeeds(st status) string {
	return fmt.Sprintf("↓ %s/s ↑ %s/s", fmtBytes(st.Traffic.DownBps), fmtBytes(st.Traffic.UpBps))
}

// barTooltip is what is behind the speeds, a fact a line: the state, the
// mode and the takeover, each route's node, and today's traffic.
func barTooltip(st status) []string {
	_, word, _ := stateCell(st.State)
	head := "bowel · " + word
	if st.Issue != nil && st.Issue.Text != "" {
		head += ": " + st.Issue.Text
	}
	tk := st.Takeover
	shown := func(e entry) string {
		if !e.Wanted {
			return entryCell("off", "").text
		}
		return entryCell(e.Status, e.Detail).text
	}
	lines := []string{
		head,
		fmt.Sprintf("%s %s · %s %s · TUN %s", i18n.T("模式", "Mode"), modeName(st.Mode),
			i18n.T("系统代理", "System proxy"), shown(tk.System), shown(tk.Tun)),
	}
	var c Ctx
	for _, r := range st.Routes {
		lines = append(lines, r.Name+" → "+c.nodeText(r).text)
	}
	lines = append(lines, fmt.Sprintf("%s ↓ %s ↑ %s", i18n.T("今日", "Today"), fmtBytes(st.Traffic.TodayDown), fmtBytes(st.Traffic.TodayUp)))
	return lines
}

// barLine is one line for the bar, in its format.
func barLine(st status, running bool, format string) string {
	class := barClass(st, running)
	text, tooltip := "bowel", []string{i18n.T("bowel 没在运行", "bowel isn't running")}
	if running {
		text, tooltip = barSpeeds(st), barTooltip(st)
	}
	if format == "text" {
		return text
	}
	classes := []string{class}
	if running && st.State != "" && st.State != "ok" {
		classes = append(classes, st.State) // degraded, error
	}
	b, _ := json.Marshal(struct {
		Text    string   `json:"text"`
		Alt     string   `json:"alt"`
		Tooltip string   `json:"tooltip"`
		Class   []string `json:"class"`
	}{pango(text), class, pango(strings.Join(tooltip, "\n")), classes})
	return string(b)
}

// pango escapes what waybar would read as markup: node and route names
// can hold & and <.
var pango = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
