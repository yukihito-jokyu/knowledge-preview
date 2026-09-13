package htmlsafe

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

func TestSanitizeAttacksAndAllowedStyles(t *testing.T) {
	sanitizer := New()
	source := `<style>body{color:red}</style><script>alert(1)</script><p id="anchor" onclick="alert(1)" style="color:red;padding:4px 8px;position:fixed;background-image:url(https://evil.test);width:expression(alert(1));--x:1">safe</p><img src="https://evil.test"><a href="https://evil.test">external</a><a href="#anchor">internal</a><svg><script>alert(2)</script></svg>`
	clean, warning, err := sanitizer.Sanitize(source)
	require.NoError(t, err)
	require.True(t, warning)

	for _, forbidden := range []string{"<script", "<style", "onclick", "<img", "<svg", "evil.test", "expression", "position", "--x"} {
		require.NotContains(t, clean, forbidden)
	}

	require.Contains(t, clean, "color: red")
	require.Contains(t, clean, "padding: 4px 8px")
	require.Contains(t, clean, `href="#anchor"`)

	for _, source := range []string{`<script>alert(1)</script>`, `<style>p{color:red}</style>`, `<img src=x>`} {
		_, _, err := sanitizer.Sanitize(source)
		require.ErrorIs(t, err, domain.ErrValidation)
	}

	for _, value := range []string{`url(https://evil.test)`, `var(--color)`, `expression(alert(1))`, `red !important`, `r\65 d`, `-1px`, `100000px`} {
		clean, _, err := sanitizer.Sanitize(`<p style="color:` + value + `;padding:` + value + `">safe</p>`)
		require.NoError(t, err)
		require.NotContains(t, clean, "style=")
	}
}
