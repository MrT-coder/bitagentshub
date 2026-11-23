package ui

import (
	"context"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/prs"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// reviewDraftMax bounds a comment being typed.
const reviewDraftMax = 2000

// diffMode is what the diff view is doing besides browsing.
type diffMode int

const (
	modeBrowse  diffMode = iota
	modeTyping           // the text of a comment is being typed
	modeConfirm          // the action waits for a y
	modeSending          // gh is sending it; keys are dropped
)

// diffAct is what the user is about to send.
type diffAct int

const (
	actLine    diffAct = iota // a comment on a line of the diff
	actApprove                // an approval
	actChanges                // a review asking for changes
	actNote                   // a note on the whole pull request
)

// lineTarget is the line a comment goes to.
type lineTarget struct {
	path string
	line int
	side prs.Side
}

// prsActionMsg is what sending a review or a comment reports back.
type prsActionMsg struct {
	act    diffAct
	repo   string
	number int
	target lineTarget
	err    error
}

// diffReviewKeys is what the diff view offers when it is idle; a review of
// your own pull request, or of one that is no longer open, is not offered.
func (m Model) diffReviewKeys() string {
	if m.reviewBlocked(actApprove) != "" {
		return i18n.T("c comment · m note · o nvim")
	}
	return i18n.T("c comment · a approve · x request changes · m note · o nvim")
}

// reviewBlocked says why act cannot be sent on the pull request of the diff
// view, or "" when it can. GitHub refuses to approve or ask changes of your
// own pull request, and of one that is merged or closed.
func (m Model) reviewBlocked(act diffAct) string {
	d := m.pulls.diff
	if act != actApprove && act != actChanges {
		return ""
	}
	switch d.pr.State {
	case "MERGED":
		return i18n.T("This pull request is already merged.")
	case "CLOSED":
		return i18n.T("This pull request is closed.")
	}
	if v := m.pulls.inbox.Viewer; v != "" && strings.EqualFold(v, d.pr.Author) {
		if act == actApprove {
			return i18n.T("You cannot approve your own pull request.")
		}
		return i18n.T("You cannot request changes on your own pull request.")
	}
	return ""
}

// diffStart begins an action: the ones that need text ask for it, an
// approval goes straight to its confirmation.
func (m Model) diffStart(act diffAct) Model {
	d := m.pulls.diff
	d.act, d.draft, d.msg = act, "", ""
	if act == actApprove {
		d.mode = modeConfirm
	} else {
		d.mode = modeTyping
	}
	return m
}

// diffComment starts a comment on the line under the cursor.
func (m Model) diffComment() Model {
	d := m.pulls.diff
	if d.cur < 0 || d.cur >= len(d.rows) || d.rows[d.cur].kind != rowLine {
		d.msg = i18n.T("Move to a code line to comment.")
		return m
	}
	r := d.rows[d.cur]
	t := lineTarget{path: d.files[r.file].Path, line: r.line.New, side: prs.Right}
	if r.line.Kind == prs.Removed {
		t.line, t.side = r.line.Old, prs.Left
	}
	m = m.diffStart(actLine)
	d.tgt = t
	return m
}

// diffTypeKey edits the text of a comment: enter moves on to the
// confirmation, esc gives up.
func (m Model) diffTypeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	d := m.pulls.diff
	d.msg = ""
	switch msg.String() {
	case "esc":
		d.mode, d.draft = modeBrowse, ""
	case "backspace":
		if r := []rune(d.draft); len(r) > 0 {
			d.draft = string(r[:len(r)-1])
		}
	case "enter":
		if strings.TrimSpace(d.draft) == "" {
			d.msg = i18n.T("A comment needs some text.")
		} else {
			d.mode = modeConfirm
		}
	default:
		if text, ok := keyText(msg); ok && utf8.RuneCountInString(d.draft+text) <= reviewDraftMax {
			d.draft += text
		}
	}
	return m, nil
}

// diffConfirmKey answers the question: only y sends, esc gives up and every
// other key is dropped, so a stray enter never publishes anything.
func (m Model) diffConfirmKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	d := m.pulls.diff
	switch msg.String() {
	case "esc":
		d.mode, d.draft = modeBrowse, ""
	case "y":
		return m.diffSend()
	}
	return m, nil
}

// diffSend sends the confirmed action off the UI thread.
func (m Model) diffSend() (Model, tea.Cmd) {
	p := m.pulls
	d := p.diff
	d.mode, d.msg = modeSending, ""
	run, act, tgt, body := p.run, d.act, d.tgt, d.draft
	repo, n, sha := d.pr.Repo, d.pr.Number, d.sha
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), prsReadTime)
		defer cancel()
		var err error
		switch act {
		case actLine:
			err = prs.PostLineComment(ctx, run, prs.LineComment{Repo: repo, Number: n, Commit: sha,
				Path: tgt.path, Line: tgt.line, Side: tgt.side, Body: body})
		case actApprove:
			err = prs.SubmitReview(ctx, run, repo, n, prs.Approve, "")
		case actChanges:
			err = prs.SubmitReview(ctx, run, repo, n, prs.RequestChanges, body)
		default:
			err = prs.SubmitReview(ctx, run, repo, n, prs.Note, body)
		}
		return prsActionMsg{act: act, repo: repo, number: n, target: tgt, err: err}
	}
}

// prsActed takes in the answer of a send. A review that went through clears
// the request on GitHub, so the inbox is read again and the star can go.
func (m Model) prsActed(msg prsActionMsg) (Model, tea.Cmd) {
	var refresh tea.Cmd
	if msg.err == nil && msg.act != actLine {
		refresh = m.prsPoll(true)
	}
	p := m.pulls
	if p == nil || p.diff == nil || p.diff.pr.Repo != msg.repo || p.diff.pr.Number != msg.number {
		return m, refresh
	}
	d := p.diff
	d.mode, d.draft = modeBrowse, ""
	switch {
	case msg.err != nil:
		d.msg = i18n.Tf("Could not send: %s", cleanText(msg.err.Error()))
	case msg.act == actApprove:
		d.msg = i18n.T("Approved.")
	case msg.act == actChanges:
		d.msg = i18n.T("Changes requested.")
	case msg.act == actNote:
		d.msg = i18n.T("Note posted.")
	default:
		d.msg = i18n.Tf("Comment posted on %s:%d.", msg.target.path, msg.target.line)
	}
	return m, refresh
}

// prsPaste puts pasted text into the filter or the comment being typed, as
// one line made safe to draw. Without one, a paste does nothing.
func (m Model) prsPaste(content string) Model {
	if m.hubTab != tabPRs || m.pulls == nil {
		return m
	}
	text := cleanText(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(content))
	switch p := m.pulls; {
	case p.editing:
		if utf8.RuneCountInString(p.draft+text) <= prsDraftMax {
			p.draft += text
		}
	case p.diff != nil && p.diff.mode == modeTyping:
		if utf8.RuneCountInString(p.diff.draft+text) <= reviewDraftMax {
			p.diff.draft += text
		}
	}
	return m
}

// drawDiffPrompt paints the row under the diff: the keys when idle, the
// text being typed, the question being asked or what happened.
func (m Model) drawDiffPrompt(buf *screen.Buffer, x, y, inner int) {
	d := m.pulls.diff
	ask := modalBorder()
	ask.Bold = true
	repo, n := d.pr.Repo, d.pr.Number
	var text string
	st := modalBorder()
	switch d.mode {
	case modeSending:
		text = d.msg
		if text == "" {
			text = i18n.T("Sending…")
		}
	case modeTyping:
		switch {
		case d.msg != "":
			text = d.msg
		case d.act == actLine:
			text = i18n.Tf("comment on %s:%d: ", d.tgt.path, d.tgt.line) + d.draft + "█"
		case d.act == actChanges:
			text = i18n.T("request changes: ") + d.draft + "█"
		default:
			text = i18n.Tf("note on %s#%d: ", repo, n) + d.draft + "█"
		}
	case modeConfirm:
		st = ask
		switch d.act {
		case actApprove:
			text = i18n.T("(y/esc) Approve this pull request?")
		case actChanges:
			text = i18n.Tf("(y/esc) Request changes: %s", d.draft)
		case actNote:
			text = i18n.Tf("(y/esc) Post this note: %s", d.draft)
		default:
			text = i18n.Tf("(y/esc) Post on %s:%d: %s", d.tgt.path, d.tgt.line, d.draft)
		}
	default:
		text, st = d.msg, modalBorder()
		if text == "" {
			text, st = m.diffReviewKeys(), modalDim()
		}
	}
	buf.Text(x, y, truncate(text, inner), st)
}
