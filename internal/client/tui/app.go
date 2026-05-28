package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/types/known/timestamppb"

	grpcclient "github.com/arsykor/gophkeeper/internal/client/grpc"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

// App is the root bubbletea model that switches between screens.
type App struct {
	client    *grpcclient.Client
	screen    Screen
	login     LoginModel
	list      ListModel
	detail    DetailModel
	create    CreateModel
	edit      EditModel
	version   string
	buildDate string
	globalErr string
}

// NewApp creates the root application model starting at the login screen.
func NewApp(client *grpcclient.Client, version, buildDate string) App {
	return App{
		client:    client,
		screen:    ScreenLogin,
		login:     NewLoginModel(client),
		version:   version,
		buildDate: buildDate,
	}
}

// Init starts the login screen.
func (a App) Init() tea.Cmd {
	return a.login.Init()
}

// Update dispatches messages to the active screen and handles screen transitions.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case msgAuthDone:
		if a.client != nil {
			a.client.SetToken(m.token)
		}
		a.screen = ScreenList
		a.list = NewListModel(a.client)
		return a, a.list.Init()

	case msgError:
		a.globalErr = m.err.Error()
		return a, nil

	case switchScreen:
		switch m.Screen {
		case ScreenList:
			a.screen = ScreenList
			a.list = NewListModel(a.client)
			return a, a.list.Init()
		case ScreenCreate:
			a.screen = ScreenCreate
			a.create = NewCreateModel(a.client)
			return a, a.create.Init()
		}

	case switchToDetail:
		a.screen = ScreenDetail
		a.detail = NewDetailModel(a.client, m.meta)
		return a, a.detail.Init()

	case switchToEdit:
		a.screen = ScreenEdit
		a.edit = NewEditModel(a.client, m.meta, m.payload)
		return a, a.edit.Init()

	case msgSaved:
		if msg.(msgSaved).err != nil {
			a.globalErr = msg.(msgSaved).err.Error()
			return a, nil
		}
		a.screen = ScreenList
		a.list = NewListModel(a.client)
		return a, a.list.Init()
	}

	a.globalErr = ""

	// Delegate to the active screen.
	switch a.screen {
	case ScreenLogin:
		newModel, cmd := a.login.Update(msg)
		a.login = newModel.(LoginModel)
		return a, cmd
	case ScreenList:
		newModel, cmd := a.list.Update(msg)
		a.list = newModel.(ListModel)
		return a, cmd
	case ScreenDetail:
		newModel, cmd := a.detail.Update(msg)
		a.detail = newModel.(DetailModel)
		return a, cmd
	case ScreenCreate:
		newModel, cmd := a.create.Update(msg)
		a.create = newModel.(CreateModel)
		return a, cmd
	case ScreenEdit:
		newModel, cmd := a.edit.Update(msg)
		a.edit = newModel.(EditModel)
		return a, cmd
	}
	return a, nil
}

// View renders the active screen plus a status bar.
func (a App) View() string {
	var content string
	switch a.screen {
	case ScreenLogin:
		content = a.login.View()
	case ScreenList:
		content = a.list.View()
	case ScreenDetail:
		content = a.detail.View()
	case ScreenCreate:
		content = a.create.View()
	case ScreenEdit:
		content = a.edit.View()
	}

	bar := subtleStyle.Render(fmt.Sprintf(" GophKeeper v%s (%s) ", a.version, a.buildDate))
	if a.globalErr != "" {
		bar += " " + errorStyle.Render(a.globalErr)
	}
	return content + "\n\n" + bar + "\n"
}

// helpers

func friendlyType(t pb.SecretType) string {
	switch t {
	case pb.SecretType_SECRET_TYPE_CREDENTIAL:
		return "credential"
	case pb.SecretType_SECRET_TYPE_TEXT:
		return "text"
	case pb.SecretType_SECRET_TYPE_BINARY:
		return "binary"
	case pb.SecretType_SECRET_TYPE_CARD:
		return "card"
	default:
		return "unknown"
	}
}

func formatTime(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().Format("2006-01-02 15:04")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

var _ = strings.Builder{}
