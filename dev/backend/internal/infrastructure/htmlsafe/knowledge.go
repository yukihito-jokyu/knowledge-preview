package htmlsafe

import (
	"html"
	"io"
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

type Sanitizer struct{ policy *bluemonday.Policy }

func New() *Sanitizer {
	p := bluemonday.NewPolicy()
	elements := strings.Fields(
		"p br hr h1 h2 h3 h4 h5 h6 div span section article header footer main nav aside blockquote pre code strong em b i u s small sub sup ul ol li dl dt dd table caption thead tbody tfoot tr th td a",
	)
	p.AllowElements(elements...)
	p.AllowAttrs("id", "class").Matching(regexp.MustCompile(`^[a-zA-Z0-9_ -]{1,200}$`)).Globally()
	p.AllowAttrs("title").Globally()
	p.AllowAttrs("lang").Matching(regexp.MustCompile(`^[a-zA-Z-]{1,35}$`)).Globally()
	p.AllowAttrs("dir").Matching(regexp.MustCompile(`^(ltr|rtl|auto)$`)).Globally()
	p.AllowAttrs("colspan", "rowspan").Matching(regexp.MustCompile(`^[1-9][0-9]{0,2}$`)).OnElements("th", "td")
	p.AllowAttrs("start").Matching(regexp.MustCompile(`^[1-9][0-9]{0,5}$`)).OnElements("ol")
	p.AllowAttrs("href").Matching(regexp.MustCompile(`^#[a-zA-Z0-9_-]+$`)).OnElements("a")
	color := regexp.MustCompile(
		`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6}|transparent|black|white|red|green|blue|gray|grey|yellow|orange|purple|pink|brown|navy|teal|silver|maroon|olive|lime|aqua|fuchsia)$`,
	)
	p.AllowStyles("color", "background-color", "border-color").Matching(color).Globally()

	length := `(?:0|(?:[0-9]{1,4})(?:\.[0-9]{1,3})?(?:px|em|rem|%))`
	p.AllowStyles("font-size", "line-height", "width", "max-width", "min-width", "height", "max-height", "min-height", "gap").
		Matching(regexp.MustCompile(`^` + length + `$`)).
		Globally()
	p.AllowStyles("margin", "padding", "border-width", "border-radius").
		Matching(regexp.MustCompile(`^` + length + `(?:\s+` + length + `){0,3}$`)).
		Globally()

	for _, name := range []string{"margin", "padding"} {
		for _, side := range []string{"top", "right", "bottom", "left"} {
			p.AllowStyles(name + "-" + side).Matching(regexp.MustCompile(`^` + length + `$`)).Globally()
		}
	}

	enums := map[string]string{
		"font-weight":     "normal|bold|[1-9]00",
		"font-style":      "normal|italic|oblique",
		"font-family":     "serif|sans-serif|monospace",
		"text-align":      "left|right|center|justify|start|end",
		"text-decoration": "none|underline|overline|line-through",
		"white-space":     "normal|pre|pre-wrap|pre-line|nowrap|break-spaces",
		"border-style":    "none|solid|dashed|dotted|double|groove|ridge|inset|outset",
		"display":         "block|inline|inline-block|flex|none",
		"flex-direction":  "row|row-reverse|column|column-reverse",
		"flex-wrap":       "nowrap|wrap|wrap-reverse",
		"justify-content": "flex-start|flex-end|center|space-between|space-around|space-evenly",
		"align-items":     "stretch|flex-start|flex-end|center|baseline",
	}
	for name, values := range enums {
		p.AllowStyles(name).Matching(regexp.MustCompile(`^(?:` + values + `)$`)).Globally()
	}

	return &Sanitizer{policy: p}
}

func (s *Sanitizer) Sanitize(source string) (string, bool, error) {
	clean := s.policy.Sanitize(stripEscapedStyles(source))

	text := html.UnescapeString(bluemonday.StrictPolicy().Sanitize(clean))
	if strings.TrimSpace(text) == "" {
		return "", false, &domain.ValidationError{
			Detail: domain.FieldError{Field: "source", Reason: "html_empty_after_sanitization"},
		}
	}

	return clean, clean != source, nil
}

// CSSパーサーはプロパティ照合前にエスケープと!importantを正規化する。
// 別表記による制限の回避も防ぐため、それらを含む属性を先に除去する。
func stripEscapedStyles(source string) string {
	tokenizer := nethtml.NewTokenizer(strings.NewReader(source))

	var output strings.Builder

	for {
		kind := tokenizer.Next()
		if kind == nethtml.ErrorToken {
			if tokenizer.Err() == io.EOF {
				return output.String()
			}

			return source
		}

		raw := string(tokenizer.Raw())
		if kind != nethtml.StartTagToken && kind != nethtml.SelfClosingTagToken {
			output.WriteString(raw)
			continue
		}

		token := tokenizer.Token()
		changed := false

		attrs := token.Attr[:0]
		for _, attr := range token.Attr {
			if attr.Key == "style" && strings.ContainsAny(attr.Val, "\\@!") {
				changed = true
				continue
			}

			attrs = append(attrs, attr)
		}

		if changed {
			token.Attr = attrs
			output.WriteString(token.String())
		} else {
			output.WriteString(raw)
		}
	}
}
