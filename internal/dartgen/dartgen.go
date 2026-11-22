// Package dartgen helps write the Dart the Flutter app (mobile/flutter)
// takes from the desktop's Go: its string literals, and each file either
// written or checked against what it would be. TestFlutterTable
// (internal/i18n) and TestFlutterSkins (the main package) use it, so that
// go generate writes the files and go test fails when they are out of date.
package dartgen

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Quote is s as a single-quoted Dart literal.
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\', '\'', '$':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u{%x}`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// Unquote is the string the body of a single-quoted Dart literal, what is
// between the quotes, stands for. A literal that interpolates is no one
// string, and is an error.
func Unquote(body string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '$' {
			return "", fmt.Errorf("'%s' interpolates", body)
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		if i++; i == len(body) {
			return "", fmt.Errorf("'%s' ends in a backslash", body)
		}
		switch c = body[i]; c {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case 'x', 'u':
			hex := body[i+1:]
			n := 2
			if c == 'u' {
				n = 4
				if strings.HasPrefix(hex, "{") {
					n = strings.IndexByte(hex, '}') + 1
				}
			}
			if n <= 0 || n > len(hex) {
				return "", fmt.Errorf("'%s' has a bad \\%c", body, c)
			}
			r, err := strconv.ParseUint(strings.Trim(hex[:n], "{}"), 16, 32)
			if err != nil {
				return "", fmt.Errorf("'%s' has a bad \\%c", body, c)
			}
			b.WriteRune(rune(r))
			i += n
		default:
			// \\, \', \" and \$, and any other character after a
			// backslash, stand for themselves.
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// Sync writes content to path when write is set. Otherwise it is an error
// unless path holds content already, line endings aside: a Windows checkout
// may have CRLF. regen is the command that writes it, for the error.
func Sync(path, content string, write bool, regen string) error {
	if write {
		return os.WriteFile(path, []byte(content), 0o644)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	have := strings.ReplaceAll(string(b), "\r\n", "\n")
	if have == content {
		return nil
	}
	h, w := strings.Split(have, "\n"), strings.Split(content, "\n")
	n := 0
	for n < len(h) && n < len(w) && h[n] == w[n] {
		n++
	}
	line := func(l []string) string {
		if n < len(l) {
			return strconv.Quote(l[n])
		}
		return "the end"
	}
	return fmt.Errorf("%s is out of date: line %d is %s, should be %s. Run %s", path, n+1, line(h), line(w), regen)
}
