// Package guide serves the operating guide and engineering standards that
// ship with pmcp (REQ-090, D-P4).
package guide

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed operating_guide.md
var operatingGuide string

//go:embed engineering_standards.md
var engineeringStandards string

var nonAnchor = regexp.MustCompile(`[^a-z0-9 -]`)

// Section is one addressable section of the served content.
type Section struct {
	URI    string
	Anchor string
	Title  string
	Body   string
}

// Anchor turns a heading into its anchor, as GitHub does.
func Anchor(title string) string {
	anchor := nonAnchor.ReplaceAllString(strings.ToLower(title), "")
	return strings.ReplaceAll(anchor, " ", "-")
}

// Sections returns every guide section (pmcp://guide/<anchor>) and standards
// section (pmcp://standards/<anchor>), in document order.
func Sections() []Section {
	return append(split("pmcp://guide/", operatingGuide), split("pmcp://standards/", engineeringStandards)...)
}

func split(prefix, document string) []Section {
	sections := []Section{}
	var current *Section
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(line, "### ") {
			if current != nil {
				sections = append(sections, *current)
			}
			title := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			current = &Section{URI: prefix + Anchor(title), Anchor: Anchor(title), Title: title}
			continue
		}
		if current != nil {
			current.Body += line + "\n"
		}
	}
	if current != nil {
		sections = append(sections, *current)
	}
	for i := range sections {
		sections[i].Body = strings.TrimSpace(sections[i].Body) + "\n"
	}
	return sections
}

// ByURI returns the section with the URI.
func ByURI(uri string) (Section, bool) {
	for _, section := range Sections() {
		if section.URI == uri {
			return section, true
		}
	}
	return Section{}, false
}

// Instructions is the server instructions text: the quick contract plus an
// index of the served sections.
func Instructions() string {
	var builder strings.Builder
	builder.WriteString("pmcp governs this repository's development life cycle. Follow the quick contract:\n\n")
	if quick, ok := ByURI("pmcp://guide/quick-contract"); ok {
		builder.WriteString(quick.Body)
	}
	builder.WriteString("\nRead the full sections as resources:\n")
	for _, section := range Sections() {
		builder.WriteString("- " + section.URI + " (" + section.Title + ")\n")
	}
	return builder.String()
}
