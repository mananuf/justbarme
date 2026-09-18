package email

import (
	"bytes"
	"fmt"
	"text/template"
)

// Brand colors for HTML email content fragments, kept in sync BY HAND with
// web/src/styles/global.css's --color-jb-* tokens -- an email client can't
// read a CSS file, so these are the one place this codebase intentionally
// duplicates that palette. If the brand tokens ever change, update both.
const (
	ColorCream     = "#F4F1E8"
	ColorInk       = "#16201A"
	ColorGreen     = "#1F3327"
	ColorGreenDark = "#121A14"
	ColorGold      = "#C9A464"
)

// logoPathData is web/src/components/Logo.tsx's own SVG path (viewBox
// "0 0 1000 1000"), copied here rather than imported since an email has no
// build step to share a frontend component through -- if the mark ever
// changes in Logo.tsx, update this to match.
const logoPathData = `M498.5,23.3C235.9,23.3,23,236.2,23,498.8v0C23,751.4,220,958,468.8,973.3v-202L108.2,425.5h768.7L524.3,770.8v202.8
C774.9,960.2,974,752.7,974,498.8V23.3H498.5z M495.3,372.9C495.2,372.9,495.2,372.9,495.3,372.9
C495.2,372.9,495.2,372.9,495.3,372.9L495.3,372.9c-218.1-26.9-191.9-216.5-191.6-218.6l0,0c0,0,0,0,0,0c0,0,0,0,0,0l0,0
C521.7,181.2,495.5,370.7,495.3,372.9L495.3,372.9z M449.2,163.5c19.5-50.3,55.7-80.3,56.5-80.9l0,0c0,0,0,0,0,0c0,0,0,0,0,0l0,0
c96.4,113.2,60,205.7,25.8,254C536.3,287.6,526.2,216.9,449.2,163.5z M696,164.6c-2.5,153-96,196.6-153.2,208.9
c17.6-16.3,84.8-86.6,58.6-186.2C648.8,163.2,695,164.6,696,164.6L696,164.6C696,164.6,696,164.6,696,164.6
C696,164.6,696.1,164.6,696,164.6L696,164.6z`

var layoutTemplate = template.Must(template.New("email_layout").Parse(layoutSrc))

// LayoutData is what Layout renders inside the shared shell.
type LayoutData struct {
	// Preheader is the hidden preview snippet most inbox lists show next
	// to the subject line -- keep it short, and let it add information
	// rather than repeat the visible content's exact wording.
	Preheader string
	// Content is the pre-rendered inner HTML for this specific email
	// (headline, body copy, an optional button). Layout itself knows
	// nothing about what it says, only how to frame it -- feature
	// packages build this with their own text/template sources.
	Content string
}

// Layout wraps Content in the shared justbarme HTML email shell -- a dark
// header band with the wordmark, a cream content card, a muted footer --
// so every transactional email looks the same regardless of which feature
// package sent it (internal/signup, internal/verification,
// internal/invitations all use this). Table-based markup with inline
// styles throughout, deliberately: HTML email has no reliable external
// stylesheet or modern-CSS support across clients, so this is written to
// the lowest common denominator rather than ported from the web app's
// Tailwind classes.
func Layout(data LayoutData) (string, error) {
	var buf bytes.Buffer
	if err := layoutTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render email layout: %w", err)
	}
	return buf.String(), nil
}

// MustLayout is Layout for package-level var initialization -- see
// MustNewTemplate. data.Content is typically itself still a raw template
// source (e.g. containing "{{.Code}}"), not yet-executed content: Layout
// only substitutes Preheader/Content into the shell textually, so a
// caller's own {{ }} placeholders inside them pass through untouched and
// become live template actions once the combined result is parsed again by
// NewHTMLTemplate. This can only fail if the shared shell template itself
// is malformed, which template.Must already catches at this package's own
// init -- so a failure here would be this codebase's bug, not a caller's.
func MustLayout(data LayoutData) string {
	html, err := Layout(data)
	if err != nil {
		panic(err)
	}
	return html
}

// headerLogoSVG and footerLogoSVG inline the brand mark at header size
// (gold, next to the wordmark) and footer size (muted ink, above the
// tagline). Email clients that don't render inline SVG at all (chiefly
// Outlook desktop's Word engine) simply show neither -- the wordmark/
// tagline text next to each still reads fine on its own, so this is an
// acceptable degradation rather than a broken email.
var headerLogoSVG = fmt.Sprintf(
	`<svg width="18" height="18" viewBox="0 0 1000 1000" xmlns="http://www.w3.org/2000/svg" style="vertical-align:middle;"><path fill="%s" d="%s"/></svg>`,
	ColorGold, logoPathData,
)

var footerLogoSVG = fmt.Sprintf(
	`<svg width="16" height="16" viewBox="0 0 1000 1000" xmlns="http://www.w3.org/2000/svg"><path fill="%s" fill-opacity="0.35" d="%s"/></svg>`,
	ColorInk, logoPathData,
)

var layoutSrc = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light">
<title>justbarme</title>
</head>
<body style="margin:0;padding:0;background-color:#121A14;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;">
<div style="display:none;max-height:0;max-width:0;overflow:hidden;opacity:0;mso-hide:all;">{{.Preheader}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#121A14;">
<tr>
<td align="center" style="padding:40px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:480px;background-color:#F4F1E8;border-radius:20px;overflow:hidden;">
<tr>
<td align="center" style="background-color:#16201A;padding:26px 32px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:0 auto;">
<tr>
<td style="padding-right:9px;">` + headerLogoSVG + `</td>
<td style="vertical-align:middle;"><span style="color:#C9A464;font-size:14px;font-weight:700;letter-spacing:0.35em;">JUSTBARME</span></td>
</tr>
</table>
</td>
</tr>
<tr>
<td style="padding:40px 32px 8px;">
{{.Content}}
</td>
</tr>
<tr>
<td style="padding:24px 32px 32px;">
<div style="border-top:1px solid rgba(22,32,26,0.1);padding-top:20px;text-align:center;">
<div style="margin-bottom:8px;">` + footerLogoSVG + `</div>
<p style="margin:0;color:rgba(22,32,26,0.4);font-size:12px;line-height:1.6;">justbarme &mdash; built for Nigerian bars.</p>
</div>
</td>
</tr>
</table>
</td>
</tr>
</table>
</body>
</html>`
