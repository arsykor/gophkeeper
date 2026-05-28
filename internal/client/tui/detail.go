package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	grpcclient "github.com/arsykor/gophkeeper/internal/client/grpc"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

// DetailModel displays the full content of a secret.
type DetailModel struct {
	client   *grpcclient.Client
	meta     *pb.SecretMeta
	payload  map[string]string
	showPass bool
	loading  bool
	status   string
}

// NewDetailModel creates the detail screen for the given secret.
func NewDetailModel(client *grpcclient.Client, meta *pb.SecretMeta) DetailModel {
	return DetailModel{client: client, meta: meta, loading: true}
}

// Init fetches the secret payload on entry.
func (m DetailModel) Init() tea.Cmd {
	client := m.client
	id := m.meta.Id
	return func() tea.Msg {
		resp, err := client.GetSecret(context.Background(), id)
		return msgSecretDetail{resp: resp, err: err}
	}
}

// Update handles key events and async results.
func (m DetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgSecretDetail:
		m.loading = false
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		var data map[string]string
		if err := json.Unmarshal(msg.resp.Payload, &data); err != nil {
			// Show raw payload for non-JSON content.
			data = map[string]string{"content": string(msg.resp.Payload)}
		}
		m.payload = data
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "b", "esc":
			return m, func() tea.Msg { return switchScreen{Screen: ScreenList} }
		case "s":
			m.showPass = !m.showPass
		case "e":
			return m, func() tea.Msg { return switchToEdit{meta: m.meta, payload: m.payload} }
		}
	}
	return m, nil
}

// View renders the detail screen.
func (m DetailModel) View() string {
	var sb strings.Builder
	sb.WriteString(boldStyle.Render(fmt.Sprintf("GophKeeper — %s", m.meta.Name)))
	sb.WriteString(fmt.Sprintf("  (%s)", friendlyType(m.meta.Type)))
	sb.WriteString("\n\n")

	if m.loading {
		sb.WriteString("  Loading secret...\n")
		return sb.String()
	}

	if m.status != "" {
		sb.WriteString("  " + errorStyle.Render(m.status) + "\n")
		sb.WriteString("\n" + subtleStyle.Render("  [B] Back"))
		return sb.String()
	}

	for k, v := range m.payload {
		val := v
		if isPasswordKey(k) && !m.showPass {
			val = strings.Repeat("•", len(v))
		}
		sb.WriteString(fmt.Sprintf("  %s: %s\n", labelStyle.Render(k), val))
	}

	if m.meta.Metadata != "" {
		sb.WriteString(fmt.Sprintf("\n  %s: %s\n", labelStyle.Render("tags"), m.meta.Metadata))
	}
	sb.WriteString(fmt.Sprintf("  %s: %s\n", labelStyle.Render("version"), fmt.Sprint(m.meta.Version)))
	sb.WriteString("\n")
	sb.WriteString(subtleStyle.Render("  [E] Edit  [S] Show/hide password  [B/Esc] Back"))
	return sb.String()
}

func isPasswordKey(k string) bool {
	k = strings.ToLower(k)
	return k == "password" || k == "pass" || k == "pin" || k == "cvv"
}

// switchToEdit carries the selected secret for editing.
type switchToEdit struct {
	meta    *pb.SecretMeta
	payload map[string]string
}
