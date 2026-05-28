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

// secretTypeNames lists the choosable types in order.
var secretTypeNames = []string{"credential", "text", "card"}

var secretTypeMap = map[string]pb.SecretType{
	"credential": pb.SecretType_SECRET_TYPE_CREDENTIAL,
	"text":       pb.SecretType_SECRET_TYPE_TEXT,
	"card":       pb.SecretType_SECRET_TYPE_CARD,
}

// CreateModel handles creating a new secret.
type CreateModel struct {
	client    *grpcclient.Client
	typeIdx   int
	nameInput textinput.Model
	metaInput textinput.Model
	fields    []textinput.Model // type-specific fields
	focused   int               // 0=name, 1=meta, 2..=fields
	status    string
}

// NewCreateModel creates the create screen.
func NewCreateModel(client *grpcclient.Client) CreateModel {
	name := textinput.New()
	name.Placeholder = "secret name"
	name.Focus()
	name.Width = 30

	meta := textinput.New()
	meta.Placeholder = "leave empty or {}"
	meta.Width = 30

	m := CreateModel{
		client:    client,
		nameInput: name,
		metaInput: meta,
	}
	m.rebuildFields()
	return m
}

func (m *CreateModel) rebuildFields() {
	typeName := secretTypeNames[m.typeIdx]
	switch typeName {
	case "credential":
		m.fields = makeFields([]string{"login", "password"}, []bool{false, true})
	case "text":
		m.fields = makeFields([]string{"content"}, []bool{false})
	case "card":
		m.fields = makeFields([]string{"number", "holder", "expiry", "cvv"}, []bool{false, false, false, true})
	}
}

func makeFields(names []string, passwords []bool) []textinput.Model {
	inputs := make([]textinput.Model, len(names))
	for i, n := range names {
		t := textinput.New()
		t.Placeholder = n
		t.Width = 30
		if passwords[i] {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		inputs[i] = t
	}
	return inputs
}

// Init implements tea.Model.
func (m CreateModel) Init() tea.Cmd { return textinput.Blink }

// Update handles key events.
func (m CreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	totalInputs := 2 + len(m.fields) // name + meta + type fields
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "b":
			return m, func() tea.Msg { return switchScreen{Screen: ScreenList} }
		case "ctrl+t":
			m.typeIdx = (m.typeIdx + 1) % len(secretTypeNames)
			m.rebuildFields()
			return m, nil
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

func (m CreateModel) refocus() (tea.Model, tea.Cmd) {
	m.nameInput.Blur()
	m.metaInput.Blur()
	for i := range m.fields {
		m.fields[i].Blur()
	}
	switch {
	case m.focused == 0:
		m.nameInput.Focus()
	case m.focused == 1:
		m.metaInput.Focus()
	default:
		m.fields[m.focused-2].Focus()
	}
	return m, nil
}

func (m CreateModel) updateInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	cmds = append(cmds, cmd)
	m.metaInput, cmd = m.metaInput.Update(msg)
	cmds = append(cmds, cmd)
	for i := range m.fields {
		m.fields[i], cmd = m.fields[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m CreateModel) save() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.status = "name is required"
		return m, nil
	}
	meta, err := normalizeMetadata(m.metaInput.Value())
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	payload := m.buildPayload()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		m.status = "failed to encode secret data"
		return m, nil
	}
	t := secretTypeMap[secretTypeNames[m.typeIdx]]
	client := m.client
	return m, func() tea.Msg {
		saved, err := client.CreateSecret(context.Background(), name, t, meta, payloadBytes)
		return msgSaved{meta: saved, err: err}
	}
}

func (m CreateModel) buildPayload() map[string]string {
	typeName := secretTypeNames[m.typeIdx]
	var keys []string
	switch typeName {
	case "credential":
		keys = []string{"login", "password"}
	case "text":
		keys = []string{"content"}
	case "card":
		keys = []string{"number", "holder", "expiry", "cvv"}
	}
	result := make(map[string]string, len(keys))
	for i, k := range keys {
		if i < len(m.fields) {
			result[k] = m.fields[i].Value()
		}
	}
	return result
}

// View renders the create form.
func (m CreateModel) View() string {
	var sb strings.Builder
	sb.WriteString(boldStyle.Render("GophKeeper — New Secret"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("  Type: %s  %s\n\n",
		greenStyle.Render(secretTypeNames[m.typeIdx]),
		subtleStyle.Render("[Ctrl+T to change]")))
	sb.WriteString("  Name:  " + m.nameInput.View() + "\n")
	sb.WriteString("  Tags:  " + m.metaInput.View() + "\n")
	sb.WriteString(subtleStyle.Render("         optional JSON — leave empty or type {}\n\n"))

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
