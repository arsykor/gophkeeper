package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	grpcclient "github.com/arsykor/gophkeeper/internal/client/grpc"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

// DetailModel displays the full content of a secret.
type DetailModel struct {
	client      *grpcclient.Client
	meta        *pb.SecretMeta
	payload     map[string]string
	showPass    bool
	loading     bool
	downloading bool
	status      string
}

// NewDetailModel creates the detail screen for the given secret.
func NewDetailModel(client *grpcclient.Client, meta *pb.SecretMeta) DetailModel {
	d := DetailModel{client: client, meta: meta}
	if meta.Type != pb.SecretType_SECRET_TYPE_BINARY {
		d.loading = true // will fetch payload
	}
	return d
}

// Init fetches the secret payload on entry (skipped for binary secrets).
func (m DetailModel) Init() tea.Cmd {
	if m.meta.Type == pb.SecretType_SECRET_TYPE_BINARY {
		return nil
	}
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

	case msgFileDownloaded:
		m.downloading = false
		if msg.err != nil {
			m.status = "Download failed: " + msg.err.Error()
		} else {
			m.status = "Saved → " + msg.path
		}
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
		case "d":
			if m.meta.Type != pb.SecretType_SECRET_TYPE_BINARY || m.downloading {
				return m, nil
			}
			m.downloading = true
			m.status = ""
			client := m.client
			id := m.meta.Id
			name := m.meta.Name
			return m, func() tea.Msg {
				_, reader, err := client.DownloadFile(context.Background(), id)
				if err != nil {
					return msgFileDownloaded{err: err}
				}
				savePath := sanitizeFilename(name)
				f, err := os.Create(savePath)
				if err != nil {
					return msgFileDownloaded{err: fmt.Errorf("cannot create file: %w", err)}
				}
				defer f.Close()
				if _, err := io.Copy(f, reader); err != nil {
					return msgFileDownloaded{err: err}
				}
				return msgFileDownloaded{path: savePath}
			}
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

	if m.meta.Type == pb.SecretType_SECRET_TYPE_BINARY {
		return m.viewBinary(&sb)
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

func (m DetailModel) viewBinary(sb *strings.Builder) string {
	sb.WriteString(fmt.Sprintf("  %s: %s\n", labelStyle.Render("size"), formatFileSize(m.meta.FileSize)))
	if m.meta.Metadata != "" {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", labelStyle.Render("tags"), m.meta.Metadata))
	}
	sb.WriteString(fmt.Sprintf("  %s: %d\n", labelStyle.Render("version"), m.meta.Version))
	sb.WriteString("\n")
	if m.downloading {
		sb.WriteString(subtleStyle.Render("  Downloading..."))
	} else {
		sb.WriteString(subtleStyle.Render("  [D] Download to current dir  [B/Esc] Back"))
	}
	if m.status != "" {
		if strings.HasPrefix(m.status, "Download failed") {
			sb.WriteString("\n  " + errorStyle.Render(m.status))
		} else {
			sb.WriteString("\n  " + greenStyle.Render(m.status))
		}
	}
	return sb.String()
}

func isPasswordKey(k string) bool {
	k = strings.ToLower(k)
	return k == "password" || k == "pass" || k == "pin" || k == "cvv"
}

func formatFileSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// sanitizeFilename replaces path separators so the secret name is safe as a local filename.
func sanitizeFilename(name string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	safe := strings.TrimSpace(r.Replace(name))
	if safe == "" {
		safe = "download"
	}
	return safe
}

// switchToEdit carries the selected secret for editing.
type switchToEdit struct {
	meta    *pb.SecretMeta
	payload map[string]string
}
