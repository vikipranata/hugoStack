package browser

import (
	"os"
	"path/filepath"
)

type Post struct {
	Path string
	Name string
}

func ListPosts(root string) ([]Post, error) {
	var posts []Post
	entries, err := os.ReadDir(filepath.Join(root, "content", "posts"))
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() {
			posts = append(posts, Post{
				Path: e.Name(),
				Name: e.Name(),
			})
		}
	}
	return posts, nil
}
