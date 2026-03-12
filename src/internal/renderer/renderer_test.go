package renderer

import (
	"strings"
	"testing"
)

func TestRenderHeading(t *testing.T) {
	html, err := Render([]byte("# Hello World"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<h1") {
		t.Errorf("expected <h1>, got:\n%s", html)
	}
}

func TestRenderTable(t *testing.T) {
	md := "| A | B |\n|---|---|\n| 1 | 2 |"
	html, err := Render([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<table") {
		t.Errorf("expected <table>, got:\n%s", html)
	}
}

func TestRenderCodeBlock(t *testing.T) {
	md := "```\nfoo bar\n```"
	html, err := Render([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<code") {
		t.Errorf("expected <code>, got:\n%s", html)
	}
}

func TestRenderInlineCode(t *testing.T) {
	html, err := Render([]byte("use `fmt.Println`"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<code>") {
		t.Errorf("expected inline <code>, got:\n%s", html)
	}
}

func TestRenderStrikethrough(t *testing.T) {
	html, err := Render([]byte("~~deleted~~"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<del>") {
		t.Errorf("expected <del>, got:\n%s", html)
	}
}

func TestRenderTaskList(t *testing.T) {
	html, err := Render([]byte("- [x] done\n- [ ] todo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `type="checkbox"`) {
		t.Errorf("expected checkbox input, got:\n%s", html)
	}
}

func TestRenderEmpty(t *testing.T) {
	html, err := Render([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<html") {
		t.Errorf("expected valid HTML document, got:\n%s", html)
	}
}

func TestRenderOutputIsFullPage(t *testing.T) {
	html, err := Render([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Errorf("expected DOCTYPE, got:\n%s", html)
	}
	if !strings.Contains(html, "</html>") {
		t.Errorf("expected closing </html>, got:\n%s", html)
	}
}
