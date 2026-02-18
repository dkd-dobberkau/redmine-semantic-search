// Package text provides utilities for preprocessing Redmine issue content
// before embedding. It strips Textile/Markdown formatting to plain text and
// splits long texts into overlapping character-based chunks.
package text

import (
	"regexp"
	"strings"
)

// Compiled regexps for Textile and Markdown formatting patterns.
// These are package-level to avoid recompilation on each call.
var (
	// rePre matches preformatted blocks and removes them entirely.
	rePre = regexp.MustCompile(`<pre>[\s\S]*?</pre>`)

	// reHTMLTag matches any remaining HTML tag.
	reHTMLTag = regexp.MustCompile(`<[^>]+>`)

	// reHeader matches Textile headers like "h1. ", "h2. ", etc.
	reHeader = regexp.MustCompile(`(?m)^h[1-6]\.\s+`)

	// reBold matches Textile bold: *word* → word (non-greedy, non-whitespace boundaries).
	reBold = regexp.MustCompile(`\*(\S[^*]*\S)\*`)

	// reItalic matches Textile italic: _word_ → word.
	reItalic = regexp.MustCompile(`_(\S[^_]*\S)_`)

	// reStrike matches Textile strikethrough: -word- → word.
	reStrike = regexp.MustCompile(`-(\S[^-]*\S)-`)

	// reCode matches Textile inline code: @word@ → word.
	reCode = regexp.MustCompile(`@([^@]+)@`)

	// reTextileLink matches Textile links: "text":url → text.
	reTextileLink = regexp.MustCompile(`"([^"]+)":(https?://\S+)`)

	// reMarkdownLink matches Markdown links: [text](url) → text.
	reMarkdownLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)

	// reMarkdownImage matches Markdown images: ![alt](url) → alt (or empty).
	reMarkdownImage = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)

	// reMultiSpace normalizes runs of whitespace to a single space.
	reMultiSpace = regexp.MustCompile(`\s{2,}`)
)

// StripMarkup converts Textile/Markdown formatted text to plain text,
// preserving readable content while removing formatting syntax.
//
// Processing order:
//  1. Remove preformatted blocks (<pre>…</pre>) — code blocks add noise
//  2. Remove remaining HTML tags
//  3. Remove Textile headers (h1. … h6.)
//  4. Extract inline formatting content (bold, italic, strikethrough, code)
//  5. Resolve links to their display text
//  6. Normalize whitespace and trim
//
// Returns empty string for empty input.
func StripMarkup(text string) string {
	if text == "" {
		return ""
	}

	// 1. Remove preformatted blocks.
	text = rePre.ReplaceAllString(text, " ")

	// 2. Remove any remaining HTML tags.
	text = reHTMLTag.ReplaceAllString(text, " ")

	// 3. Remove Textile headers (h1. … h6.) at line start.
	text = reHeader.ReplaceAllString(text, "")

	// 4. Extract inline formatting — keep the inner text, drop the markers.
	text = reBold.ReplaceAllString(text, "$1")
	text = reItalic.ReplaceAllString(text, "$1")
	text = reStrike.ReplaceAllString(text, "$1")
	text = reCode.ReplaceAllString(text, "$1")

	// 5. Resolve links to their display text.
	text = reMarkdownImage.ReplaceAllString(text, "$1")
	text = reTextileLink.ReplaceAllString(text, "$1")
	text = reMarkdownLink.ReplaceAllString(text, "$1")

	// 6. Normalize whitespace and trim.
	text = reMultiSpace.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}
