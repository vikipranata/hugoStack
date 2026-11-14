package browser

import (
	"os"
	"path/filepath"
	"sort"
)

// Post is one page-bundle dir under content/posts/<slug>/.
type Post struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	HasEN bool   `json:"has_en"`
	HasID bool   `json:"has_id"`
}

// ListPosts scans <root>/content/posts and reports, per post dir, which
// language files exist (index.md / index.id.md). Title comes from the
// frontmatter of index.md, falling back to index.id.md, then the slug.
func ListPosts(root string) ([]Post, error) {
	entries, err := os.ReadDir(filepath.Join(root, "content", "posts"))
	if err != nil {
		return nil, err
	}
	posts := make([]Post, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, "content", "posts", e.Name())
		p := Post{Slug: e.Name()}
		var title string
		for _, f := range []string{"index.md", "index.id.md"} {
			if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
				if f == "index.md" {
					p.HasEN = true
				} else {
					p.HasID = true
				}
				if title == "" {
					title = TitleOf(string(b))
				}
			}
		}
		if title == "" {
			title = e.Name()
		}
		p.Title = title
		posts = append(posts, p)
	}
	sort.Slice(posts, func(i, j int) bool { return posts[i].Slug < posts[j].Slug })
	return posts, nil
}
