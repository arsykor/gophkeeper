package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	grpcclient "github.com/arsykor/gophkeeper/internal/client/grpc"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

// ListModel shows the list of secrets (metadata only).
type ListModel struct {
	client  *grpcclient.Client
	secrets []*pb.SecretMeta
	cursor  int
	status  string
	loading bool
}

// NewListModel creates the list screen.
func NewListModel(client *grpcclient.Client) ListModel {
	return ListModel{client: client, loading: true}
}

// Init fetches secrets on startup.
func (m ListModel) Init() tea.Cmd {
	return m.fetchSecrets()
}

func (m ListModel) fetchSecrets() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		secrets, err := client.ListSecrets(context.Background())
		return msgSecretsLoaded{secrets: secrets, err: err}
	}
}

// Update handles keypresses and async results.
func (m ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgSecretsLoaded:
		m.loading = false
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
		} else {
			m.secrets = msg.secrets
			m.status = ""
		}
		return m, nil

	case msgDeleted:
		if msg.err != nil {
			m.status = "Delete failed: " + msg.err.Error()
			return m, nil
		}
		m.status = "Deleted."
		m.loading = true
		return m, m.fetchSecrets()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.secrets)-1 {
				m.cursor++
			}
		case "r":
			m.loading = true
			return m, m.fetchSecrets()
		case "n":
			return m, func() tea.Msg { return switchScreen{Screen: ScreenCreate} }
		case "d":
			if len(m.secrets) == 0 {
				return m, nil
			}
			id := m.secrets[m.cursor].Id
			client := m.client
			return m, func() tea.Msg {
				err := client.DeleteSecret(context.Background(), id)
				return msgDeleted{err: err}
			}
		case "enter":
			if len(m.secrets) == 0 {
				return m, nil
			}
			selected := m.secrets[m.cursor]
			return m, func() tea.Msg { return switchToDetail{meta: selected} }
		}
	}
	return m, nil
}

// View renders the list screen.
func (m ListModel) View() string {
	var sb strings.Builder
	sb.WriteString(boldStyle.Render("GophKeeper — Secrets"))
	sb.WriteString("\n\n")

	if m.loading {
		sb.WriteString("  Loading...\n")
		return sb.String()
	}

	if len(m.secrets) == 0 {
		sb.WriteString(subtleStyle.Render("  No secrets yet. Press [N] to create one."))
		sb.WriteString("\n")
	} else {
		sb.WriteString(fmt.Sprintf("  %-30s %-12s %-20s\n",
			labelStyle.Render("NAME"), labelStyle.Render("TYPE"), labelStyle.Render("UPDATED")))
		sb.WriteString("  " + strings.Repeat("─", 64) + "\n")
		for i, s := range m.secrets {
			line := fmt.Sprintf("%-30s %-12s %-20s",
				truncate(s.Name, 28),
				friendlyType(s.Type),
				formatTime(s.UpdatedAt),
			)
			if i == m.cursor {
				sb.WriteString("  " + selectedStyle.Render("> "+line) + "\n")
			} else {
				sb.WriteString("    " + line + "\n")
			}
		}
	}

	sb.WriteString("\n")
	sb.WriteString(subtleStyle.Render("  [↑/↓] Navigate  [Enter] View  [N] New  [D] Delete  [R] Refresh  [Q] Quit"))
	if m.status != "" {
		sb.WriteString("\n  " + errorStyle.Render(m.status))
	}
	return sb.String()
}

// switchScreen is sent when navigating to a new screen without data.
type switchScreen struct{ Screen Screen }

// switchToDetail carries the selected secret metadata.
type switchToDetail struct{ meta *pb.SecretMeta }
