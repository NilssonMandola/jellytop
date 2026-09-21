package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NilssonMandola/jellytop/internal/jellyfin"
	tea "github.com/charmbracelet/bubbletea"
)

type usersMsg struct {
	items []jellyfin.User
	err   error
}

type usersPane struct {
	client *jellyfin.Client
	items  []jellyfin.User
	cur    cursor
	err    error
}

func newUsersPane(c *jellyfin.Client) *usersPane { return &usersPane{client: c} }

func (p *usersPane) Title() string           { return "Users" }
func (p *usersPane) Interval() time.Duration { return 60 * time.Second }
func (p *usersPane) Err() error              { return p.err }

func (p *usersPane) Load() tea.Cmd {
	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		items, err := c.Users(ctx)
		return usersMsg{items: items, err: err}
	}
}

func (p *usersPane) Summary() string {
	admins, disabled := 0, 0
	for _, u := range p.items {
		if u.Policy == nil {
			continue
		}
		if u.Policy.IsAdministrator {
			admins++
		}
		if u.Policy.IsDisabled {
			disabled++
		}
	}
	return fmt.Sprintf("%d users · %d admin · %d disabled", len(p.items), admins, disabled)
}

func (p *usersPane) Keys() []keyHint {
	return []keyHint{
		{"d", "enable/disable"},
		{"a", "toggle admin"},
		{"r", "refresh"},
	}
}

func (p *usersPane) selected() (jellyfin.User, bool) {
	if p.cur.idx < 0 || p.cur.idx >= len(p.items) {
		return jellyfin.User{}, false
	}
	return p.items[p.cur.idx], true
}

func (p *usersPane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case usersMsg:
		p.err = msg.err
		if msg.err == nil {
			p.items = msg.items
			p.cur.move(0, len(p.items))
		}
		return nil, true

	case tea.KeyMsg:
		u, ok := p.selected()
		if !ok || u.Policy == nil {
			return nil, false
		}
		c := p.client
		switch msg.String() {
		case "d":
			disable := !u.Policy.IsDisabled
			verb := "Disable"
			if !disable {
				verb = "Re-enable"
			}
			return confirm(fmt.Sprintf("%s account %q?", verb, u.Name), disable, func() tea.Cmd {
				return runAction(fmt.Sprintf("%s: %sd", u.Name, strings.ToLower(verb)), func(ctx context.Context) error {
					return c.SetUserDisabled(ctx, u.ID, disable)
				}, p.Load())
			}), true
		case "a":
			admin := !u.Policy.IsAdministrator
			verb := "Grant admin to"
			if !admin {
				verb = "Revoke admin from"
			}
			return confirm(fmt.Sprintf("%s %q?", verb, u.Name), true, func() tea.Cmd {
				return runAction(fmt.Sprintf("%s: admin=%v", u.Name, admin), func(ctx context.Context) error {
					return c.SetUserAdmin(ctx, u.ID, admin)
				}, p.Load())
			}), true
		}
	}
	return nil, false
}

func (p *usersPane) MoveCursor(delta int) { p.cur.move(delta, len(p.items)) }
func (p *usersPane) Home()                { p.cur.toTop() }
func (p *usersPane) End()                 { p.cur.toEnd(len(p.items)) }

func (p *usersPane) View(w, h int) string {
	if len(p.items) == 0 {
		return emptyState("No users.", w, h)
	}

	const (
		wName  = 24
		wState = 10
		wRole  = 7
		wLogin = 14
		wSeen  = 14
	)
	wID := w - (wName + wState + wRole + wLogin + wSeen + 5)
	if wID < 8 {
		wID = 8
	}

	var b strings.Builder
	b.WriteString(renderRow(false,
		col("USER", wName, styleHeaderRow), col("STATE", wState, styleHeaderRow),
		col("ROLE", wRole, styleHeaderRow), col("LAST LOGIN", wLogin, styleHeaderRow),
		col("LAST SEEN", wSeen, styleHeaderRow), col("ID", wID, styleHeaderRow),
	))
	b.WriteString("\n")

	lo, hi := p.cur.window(len(p.items), h-1)
	for i := lo; i < hi; i++ {
		u := p.items[i]

		state, role := col("enabled", wState, styleOK), col("user", wRole, styleMuted)
		if u.Policy != nil {
			if u.Policy.IsDisabled {
				state = col("disabled", wState, styleErr)
			}
			if u.Policy.IsAdministrator {
				role = col("admin", wRole, styleWarn)
			}
		}

		b.WriteString(renderRow(i == p.cur.idx,
			col(u.Name, wName, styleText), state, role,
			col(relTimePtr(u.LastLoginDate), wLogin, styleMuted),
			col(relTimePtr(u.LastActivityDate), wSeen, styleMuted),
			col(u.ID, wID, styleFaint),
		))
		if i < hi-1 {
			b.WriteString("\n")
		}
	}
	return fillHeight(b.String(), h)
}

func relTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "never"
	}
	return relTime(*t) + " ago"
}
