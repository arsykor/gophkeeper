package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	grpcclient "github.com/arsykor/gophkeeper/internal/client/grpc"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

// EditModel lets the user modify an existing secret.
type EditModel struct {
	client    *grpcclient.Client
	meta      *pb.SecretMeta
	metaInput textinput.Model
	fields    []textinput.Model
	keys      []string
	focused   int
	status    string
}

// NewEditModel creates the edit screen pre-filled with existing values.
func NewEditModel(client *grpcclient.Client, meta *pb.SecretMeta, payload map[string]string) EditModel {
	metaIn := textinput.New()
	metaIn.SetValue(meta.Metadata)
	metaIn.Placeholder = "leave empty or {}"
	metaIn.Width = 30

	keys := keysForType(meta.Type)
	fields := make([]textinput.Model, len(keys))
	for i, k := range keys {
		t := textinput.New()
		t.Placeholder = k
		t.Width = 30
		t.SetValue(payload[k])
		if isPasswordKey(k) {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		fields[i] = t
	}
	if len(fields) > 0 {
		fields[0].Focus()
	}

	return EditModel{
		client:    client,
		meta:      meta,
		metaInput: metaIn,
		fields:    fields,
		keys:      keys,
	}
}

func keysForType(t pb.SecretType) []string {
	switch t {
	case pb.SecretType_SECRET_TYPE_CREDENTIAL:
		return []string{"login", "password"}
	case pb.SecretType_SECRET_TYPE_TEXT:
		return []string{"content"}
	case pb.SecretType_SECRET_TYPE_CARD:
		return []string{"number", "holder", "expiry", "cvv"}
	default:
		return []string{"content"}
	}
}

// Init implements tea.Model.
func (m EditModel) Init() tea.Cmd { return textinput.Blink }

// Update handles key events.
func (m EditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	totalInputs := 1 + len(m.fields)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "b":
			return m, func() tea.Msg { return switchScreen{Screen: ScreenList} }
		case "tab", "down":
			m.focused = (m.focused + 1) % totalInputs
			return m.refocus()
		case "shift+tab", "up":
			m.focused = (m.focused - 1 + totalInputs) % totalInputs
			return m.refocus()
		case "enter":
			if m.focused < totalInputs-1 {
				m.focused++
				return m.refocus()
			}
			return m.save()
		}
	}
	return m.updateInputs(msg)
}

func (m EditModel) refocus() (tea.Model, tea.Cmd) {
	m.metaInput.Blur()
	for i := range m.fields {
		m.fields[i].Blur()
	}
	if m.focused == 0 {
		m.metaInput.Focus()
	} else {
		m.fields[m.focused-1].Focus()
	}
	return m, nil
}

func (m EditModel) updateInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.metaInput, cmd = m.metaInput.Update(msg)
	cmds = append(cmds, cmd)
	for i := range m.fields {
		m.fields[i], cmd = m.fields[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m EditModel) save() (tea.Model, tea.Cmd) {
	data := make(map[string]string, len(m.keys))
	for i, k := range m.keys {
		data[k] = m.fields[i].Value()
	}
	metaStr, err := normalizeMetadata(m.metaInput.Value())
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	payloadBytes, err := json.Marshal(data)
	if err != nil {
		m.status = "failed to encode secret data"
		return m, nil
	}
	client := m.client
	id := m.meta.Id
	name := m.meta.Name
	version := m.meta.Version
	return m, func() tea.Msg {
		saved, err := client.UpdateSecret(context.Background(), id, name, metaStr, payloadBytes, version)
		return msgSaved{meta: saved, err: err}
	}
}

// View renders the edit form.
func (m EditModel) View() string {
	var sb strings.Builder
	sb.WriteString(boldStyle.Render(fmt.Sprintf("GophKeeper — Edit: %s", m.meta.Name)))
	sb.WriteString("\n\n")
	sb.WriteString("  Tags:  " + m.metaInput.View() + "\n\n")
	for i, f := range m.fields {
		_ = i
		sb.WriteString(fmt.Sprintf("  %-10s %s\n", f.Placeholder+":", f.View()))
	}
	sb.WriteString("\n")
	sb.WriteString(subtleStyle.Render("  [Tab] Next  [Enter] Save  [B/Esc] Cancel"))
	if m.status != "" {
		sb.WriteString("\n  " + errorStyle.Render(m.status))
	}
	return sb.String()
}
