package chatgptweb

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var dataBuildPattern = regexp.MustCompile(`c/[^/]*/_`)

func ParsePowResources(htmlContent string) (scriptSources []string, dataBuild string) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return []string{DefaultPowScript}, ""
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "html":
				for _, a := range n.Attr {
					if a.Key == "data-build" && a.Val != "" && dataBuild == "" {
						dataBuild = a.Val
					}
				}
			case "script":
				for _, a := range n.Attr {
					if a.Key == "src" && a.Val != "" {
						scriptSources = append(scriptSources, a.Val)
						if m := dataBuildPattern.FindString(a.Val); m != "" && dataBuild == "" {
							dataBuild = m
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if len(scriptSources) == 0 {
		scriptSources = []string{DefaultPowScript}
	}
	return
}
