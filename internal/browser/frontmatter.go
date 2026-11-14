package browser

import (
	"errors"
	"strings"
)

// Frontmatter is the editable subset of a post's YAML frontmatter.
type Frontmatter struct {
	Title       string
	Tags        []string
	Description string
}

// splitFrontmatter splits raw post content into the frontmatter block
// (both "---" delimiter lines included) and the body after it.
// Split/join on "\n" only so "\r" line endings survive untouched.
func Split(content string) (fm, body string, ok bool) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return "", "", false
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSuffix(lines[i], "\r") == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return "", "", false
	}
	return strings.Join(lines[:closeIdx+1], "\n"), strings.Join(lines[closeIdx+1:], "\n"), true
}

func yamlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", " ") // ponytail: single-line values only; block scalars if multi-line titles ever appear
	return `"` + s + `"`
}

func tagsInline(tags []string) string {
	parts := make([]string, 0, len(tags))
	for _, t := range tags {
		if isPlainYamlScalar(t) {
			parts = append(parts, t)
		} else {
			parts = append(parts, yamlString(t))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func isPlainYamlScalar(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		if i > 0 && (r == '-' || r == '_' || r == '.' || r == '/') {
			continue
		}
		return false
	}
	return true
}

// Rewrite returns content with title/tags/description updated in the
// frontmatter and the body replaced by newBody. Every other frontmatter
// line (date, modified, IDtags, ...) is preserved verbatim, along with
// the file's original line ending.
func Rewrite(content string, fm Frontmatter, newBody string) (string, error) {
	fmRaw, _, ok := Split(content)
	if !ok {
		return "", errors.New("post has no YAML frontmatter")
	}
	lines := strings.Split(fmRaw, "\n")
	out := []string{strings.TrimSuffix(lines[0], "\r")}
	found := map[string]bool{}
	for i := 1; i < len(lines)-1; i++ {
		l := strings.TrimSuffix(lines[i], "\r")
		switch {
		case strings.HasPrefix(l, "title:"):
			out = append(out, "title: "+yamlString(fm.Title))
			found["title"] = true
		case strings.HasPrefix(l, "description:"):
			out = append(out, "description: "+yamlString(fm.Description))
			found["description"] = true
		case strings.HasPrefix(l, "tags:"):
			out = append(out, "tags: "+tagsInline(fm.Tags))
			found["tags"] = true
			for i+1 < len(lines)-1 && isListLine(strings.TrimSuffix(lines[i+1], "\r")) {
				i++ // drop block-list continuation of the old tags value
			}
		default:
			out = append(out, l)
		}
	}
	if !found["title"] {
		out = append(out, "title: "+yamlString(fm.Title))
	}
	if !found["tags"] {
		out = append(out, "tags: "+tagsInline(fm.Tags))
	}
	if !found["description"] {
		out = append(out, "description: "+yamlString(fm.Description))
	}
	out = append(out, "---")
	eol := "\n"
	if strings.Contains(fmRaw, "\r\n") {
		eol = "\r\n"
	}
	return strings.Join(out, eol) + eol + newBody, nil
}

func isListLine(l string) bool {
	trimmed := strings.TrimLeft(l, " \t")
	return len(trimmed) < len(l) && strings.HasPrefix(trimmed, "- ")
}

func Unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	return s
}

func FMField(content, key string) (string, bool) {
	fm, _, ok := Split(content)
	if !ok {
		return "", false
	}
	lines := strings.Split(fm, "\n")
	for i := 1; i < len(lines)-1; i++ {
		l := strings.TrimSuffix(lines[i], "\r")
		if rest, ok2 := strings.CutPrefix(l, key+":"); ok2 {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

// TitleOf extracts the frontmatter title value ("").
func TitleOf(content string) string {
	v, _ := FMField(content, "title")
	return Unquote(v)
}

// TagsOf extracts the frontmatter tags (inline "[a, b]" or "- a" block list).
func TagsOf(content string) []string {
	fm, _, ok := Split(content)
	if !ok {
		return nil
	}
	lines := strings.Split(fm, "\n")
	for i := 1; i < len(lines)-1; i++ {
		l := strings.TrimSuffix(lines[i], "\r")
		rest, ok2 := strings.CutPrefix(l, "tags:")
		if !ok2 {
			continue
		}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "[") {
			return splitInlineList(strings.TrimSuffix(strings.TrimPrefix(rest, "["), "]"))
		}
		var tags []string
		for i+1 < len(lines)-1 {
			nl := strings.TrimSuffix(lines[i+1], "\r")
			if !isListLine(nl) {
				break
			}
			item := strings.TrimSpace(strings.TrimPrefix(strings.TrimLeft(nl, " \t"), "-"))
			tags = append(tags, Unquote(item))
			i++
		}
		return tags
	}
	return nil
}

func splitInlineList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(Unquote(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
