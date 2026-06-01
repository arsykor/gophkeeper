package tui

import "github.com/charmbracelet/lipgloss"

var (
	boldStyle   = lipgloss.NewStyle().Bold(true)
	subtleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
)
