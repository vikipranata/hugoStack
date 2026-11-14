package browser

import (
	"strings"
	"testing"
)

func TestRewriteCRLFPreservedAndFieldsKept(t *testing.T) {
	orig := "---\r\ntitle: \"Old Title\"\r\ndate: 2025-03-15 00:00:00 +0700\r\nmodified: 2025-03-15 00:00:00 +0700\r\ntags: [linux, ceph, storage]\r\ndescription: \"Old Desc\"\r\nIDtags: [linux, ceph, storage]\r\n---\r\n\r\nold body line\r\n"
	out, err := Rewrite(orig, Frontmatter{Title: "New Title", Tags: []string{"a", "b"}, Description: "New Desc"}, "new body\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"title: \"New Title\"\r\n",
		"date: 2025-03-15 00:00:00 +0700\r\n",
		"modified: 2025-03-15 00:00:00 +0700\r\n",
		"tags: [a, b]\r\n",
		"description: \"New Desc\"\r\n",
		"IDtags: [linux, ceph, storage]\r\n",
		"---\r\nnew body\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "old body line") || strings.Contains(out, "Old Title") {
		t.Errorf("old content leaked:\n%s", out)
	}
}

func TestRewriteLFBlockTags(t *testing.T) {
	orig := "---\ntitle: 'T'\ntags:\n  - one\n  - two\ndescription: D\n---\nbody"
	out, err := Rewrite(orig, Frontmatter{Title: "T2", Tags: []string{"x"}, Description: "D2"}, "b2")
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: \"T2\"\ntags: [x]\ndescription: \"D2\"\n---\nb2"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestParse(t *testing.T) {
	c := "---\r\ntitle: \"Ceph Reef Deployment\"\r\ntags: [linux, ceph, storage]\r\ndescription: \"Desc\"\r\n---\r\nbody"
	if got := TitleOf(c); got != "Ceph Reef Deployment" {
		t.Errorf("title = %q", got)
	}
	tags := TagsOf(c)
	if len(tags) != 3 || tags[0] != "linux" || tags[2] != "storage" {
		t.Errorf("tags = %v", tags)
	}
	if _, _, ok := Split("no frontmatter"); ok {
		t.Error("should not parse")
	}
}
