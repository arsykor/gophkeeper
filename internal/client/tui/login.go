package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	grpcclient "github.com/arsykor/gophkeeper/internal/client/grpc"
)

// LoginModel is the login / register screen.
type LoginModel struct {
	client   *grpcclient.Client
	inputs   []textinput.Model
	focused  int
	register bool // true = register mode
	err      string
}

// NewLoginModel creates the login screen model.
func NewLoginModel(client *grpcclient.Client) LoginModel {
	login := textinput.New()
	login.Placeholder = "username"
	login.Focus()
	login.CharLimit = 64
	login.Width = 30

	pass := textinput.New()
	pass.Placeholder = "password"
	pass.EchoMode = textinput.EchoPassword
	pass.EchoCharacter = '•'
	pass.CharLimit = 64
	pass.Width = 30

	return LoginModel{
		client:  client,
		inputs:  []textinput.Model{login, pass},
		focused: 0,
	}
}

// Init implements tea.Model.
func (m LoginModel) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m LoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "down":
			m.focused = (m.focused + 1) % len(m.inputs)
			return m.refocus()
		case "shift+tab", "up":
			m.focused = (m.focused - 1 + len(m.inputs)) % len(m.inputs)
			return m.refocus()
		case "ctrl+r":
			m.register = !m.register
			m.err = ""
			return m, nil
		case "enter":
			if m.focused < len(m.inputs)-1 {
				m.focused++
				return m.refocus()
			}
			return m.submit()
		}
	}

	var cmds []tea.Cmd
	for i := range m.inputs {
		var cmd tea.Cmd
		m.inputs[i], cmd = m.inputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m LoginModel) refocus() (tea.Model, tea.Cmd) {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	m.inputs[m.focused].Focus()
	return m, nil
}

func (m LoginModel) submit() (tea.Model, tea.Cmd) {
	login := strings.TrimSpace(m.inputs[0].Value())
	password := m.inputs[1].Value()
	if login == "" || password == "" {
		m.err = "login and password are required"
		return m, nil
	}
	client := m.client
	reg := m.register
	return m, func() tea.Msg {
		var (
			token string
			err   error
		)
		if reg {
			token, err = client.Register(context.Background(), login, password)
		} else {
			token, err = client.Login(context.Background(), login, password)
		}
		if err != nil {
			return msgError{err: err}
		}
		return msgAuthDone{token: token}
	}
}

// View implements tea.Model.
func (m LoginModel) View() string {
	var sb strings.Builder
	title := "Login"
	if m.register {
		title = "Register"
	}
	sb.WriteString(boldStyle.Render("GophKeeper — " + title))
	sb.WriteString("\n\n")
	sb.WriteString("  Username: " + m.inputs[0].View() + "\n")
	sb.WriteString("  Password: " + m.inputs[1].View() + "\n\n")
	if m.register {
		sb.WriteString(subtleStyle.Render("  [Ctrl+R] Switch to Login  [Enter] Register  [Tab] Next field"))
	} else {
		sb.WriteString(subtleStyle.Render("  [Ctrl+R] Switch to Register  [Enter] Login  [Tab] Next field"))
	}
	if m.err != "" {
		sb.WriteString("\n\n  " + errorStyle.Render(m.err))
	}
	return sb.String()
}
