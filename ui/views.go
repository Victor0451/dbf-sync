package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"dbf-sync/config"
)

// View returns the appropriate view based on the current state
func (m *AppModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("\n\n   Cargando...")
		v.AltScreen = true
		return v
	}

	var content string

	switch m.state {
	case StateMainMenu:
		content = m.viewMainMenu()
	case StateSelectDB:
		content = m.viewSelectDB()
	case StateSelectTable:
		content = m.viewSelectTable()
	case StateSelectAction:
		content = m.viewSelectAction()
	case StateBrowseFile:
		content = m.viewBrowseFile()
	case StateManualPath:
		content = m.viewManualPath()
	case StateInputMonth:
		content = m.viewInputMonth()
	case StateInputYear:
		content = m.viewInputYear()
	case StateConfirm:
		content = m.viewConfirm()
	case StateProcessing:
		content = m.viewProcessing()
	case StateSummary:
		content = m.viewSummary()
	case StateStatus:
		content = m.viewStatus()
	case StateInstalling:
		content = m.viewInstalling()
	case StateInstallDone:
		content = m.viewInstallDone()
	case StateConfig:
		content = m.viewConfig()
	case StateQuit:
		content = m.viewQuit()
	default:
		content = m.viewMainMenu()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// viewMainMenu shows the main menu
func (m *AppModel) viewMainMenu() string {
	if m.list.Items() == nil || len(m.list.Items()) == 0 {
		m.loadMainMenu()
	}

	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
     ██████╗ ██████╗ ███████╗    ███████╗██╗   ██╗███╗   ██╗ ██████╗ ██████╗ ███████╗
     ██╔══██╗██╔══██╗██╔════╝    ██╔════╝╚██╗ ██╔╝████╗  ██║██╔════╝ ██╔══██╗██╔════╝
     ██║  ██║██████╔╝█████╗      ███████╗ ╚████╔╝ ██╔██╗ ██║██║  ███╗██████╔╝█████╗
     ██║  ██║██╔══██╗██╔══╝      ╚════██║  ╚██╔╝  ██║╚██╗██║██║   ██║██╔══██╗██╔══╝
     ██████╔╝██║  ██║██║         ███████║   ██║   ██║ ╚████║╚██████╔╝██║  ██║███████╗
     ╚═════╝ ╚═╝  ╚═╝╚═╝         ╚══════╝   ╚═╝   ╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝

                      DBF Sync %s — Modo Interactivo

                        Powered by VML PROGRAMMING

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Que queres hacer?
`, m.version))

	menuView := m.list.View()
	
	footer := HintStyle.Width(m.width - 4).Render(`
  ↑/↓ navegar · Enter confirmar · q salir`)

	return header + "\n" + menuView + "\n" + footer
}

// viewSelectDB shows database selection
func (m *AppModel) viewSelectDB() string {
	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Seleccionar Base de Datos

  ↑/↓ navegar · Enter confirmar · Esc volver`))

	menuView := m.list.View()

	footer := HintStyle.Width(m.width - 4).Render(fmt.Sprintf("\n  ► Base de datos: %s", m.db))

	return header + "\n" + menuView + "\n" + footer
}

// viewSelectTable shows table selection with loading state
func (m *AppModel) viewSelectTable() string {
	if m.loading {
		header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Conectando a MySQL (%s)...

  %s`, m.loadingMsg, m.spinner.View()))

		return header
	}

	if m.err != nil {
		header := ErrorStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Error: %v

  Presioná cualquier tecla para continuar...`, m.err))

		return header
	}

	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Seleccionar Tabla (%s)

  ↑/↓ navegar · Enter confirmar · Esc volver`, m.db))

	menuView := m.list.View()

	return header + "\n" + menuView
}

// viewSelectAction shows action selection
func (m *AppModel) viewSelectAction() string {
	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Seleccionar Acción

  ► Base de datos: %s
  ► Tabla: %s

  ↑/↓ navegar · Enter confirmar · Esc volver`, m.db, m.table))

	menuView := m.list.View()

	return header + "\n" + menuView
}

// viewBrowseFile shows the file browser
func (m *AppModel) viewBrowseFile() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Width(m.width - 4).Render(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Seleccionar Archivo .dbf

  ► Base de datos: ` + m.db + "\n"))
	b.WriteString("  ► Tabla: " + m.table + "\n")
	b.WriteString("  ► Acción: " + getActionLabel(m.action) + "\n")
		b.WriteString("\n  📁 " + m.currentDir + "\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if len(m.dirEntries) == 0 {
		b.WriteString(HintStyle.Render("  No hay archivos .dbf en este directorio\n"))
	} else {
		for i, entry := range m.dirEntries {
			icon := "📄 "
			if entry.IsDir {
				icon = "📁 "
			}
			
			cursor := "  "
			var nameStr string
			if i == m.cursor {
				cursor = "► "
				nameStr = SelectedStyle.Render(entry.Name)
			} else {
				nameStr = entry.Name
			}

			if entry.IsDir {
				b.WriteString(fmt.Sprintf("  %s%s%s\n", cursor, icon, nameStr))
			} else {
				b.WriteString(fmt.Sprintf("  %s%s%s %s\n", cursor, icon, nameStr, MutedStyle(FormatSize(entry.Size))))
			}
		}
	}

	b.WriteString(HintStyle.Width(m.width - 4).Render(`
  ↑/↓ navegar · Enter seleccionar · Backspace subir · t manual · Esc volver`))

	return b.String()
}

// viewManualPath shows manual path input
func (m *AppModel) viewManualPath() string {
	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Ingresar Ruta Manualmente

  ► Base de datos: %s
  ► Tabla: %s
  ► Acción: %s

`, m.db, m.table, getActionLabel(m.action)))

	inputView := m.textInput.View()

	footer := HintStyle.Width(m.width - 4).Render(`
  Enter confirmar · Esc cancelar`)

	return header + "\n" + inputView + "\n" + footer
}

// viewInputMonth shows month input
func (m *AppModel) viewInputMonth() string {
	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Ingresar Mes

  ► Base de datos: %s
  ► Tabla: %s
  ► Archivo: %s
  ► Acción: Actualizar cobradores

`, m.db, m.table, m.dbfPath))

	inputView := m.textInput.View()

	footer := HintStyle.Width(m.width - 4).Render(`
  Ingresá el mes (1-12) · Enter confirmar · Esc volver`)

	if m.err != nil {
		footer = ErrorStyle.Width(m.width - 4).Render(m.err.Error()) + "\n" + footer
	}

	return header + "\n" + "  Mes: " + inputView + "\n\n" + footer
}

// viewInputYear shows year input
func (m *AppModel) viewInputYear() string {
	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Ingresar Año

  ► Base de datos: %s
  ► Tabla: %s
  ► Archivo: %s
  ► Mes: %d
  ► Acción: Actualizar cobradores

`, m.db, m.table, m.dbfPath, m.month))

	inputView := m.textInput.View()

	footer := HintStyle.Width(m.width - 4).Render(`
  Ingresá el año (e.g. 2026) · Enter confirmar · Esc volver`)

	if m.err != nil {
		footer = ErrorStyle.Width(m.width - 4).Render(m.err.Error()) + "\n" + footer
	}

	return header + "\n" + "  Año: " + inputView + "\n\n" + footer
}

// viewConfirm shows confirmation dialog
func (m *AppModel) viewConfirm() string {
	period := ""
	if m.action == "cobrador" {
		period = fmt.Sprintf("  ► Período: %02d/%d\n", m.month, m.year)
	}

	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Confirmar Sincronización

  ► Base de datos: %s
  ► Tabla: %s
  ► Archivo: %s
%s  ► Acción: %s

  ¿Confirmar? (s/n)

`, m.db, m.table, m.dbfPath, period, getActionLabel(m.action)))

	footer := HintStyle.Width(m.width - 4).Render(`
  s/Sí · n/No · Esc volver`)

	return header + footer
}

// viewProcessing shows processing state
func (m *AppModel) viewProcessing() string {
	period := ""
	if m.action == "cobrador" && m.month > 0 {
		period = fmt.Sprintf("\n  Periodo:        %02d/%d", m.month, m.year)
	}

	return TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  %s  Sincronizando...

  Base de datos:  %s
  Tabla:          %s
  Accion:         %s%s

  Este proceso puede tardar varios segundos.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`,
		m.spinner.View(),
		m.db,
		m.table,
		getActionLabel(m.action),
		period,
	))
}

// viewSummary shows sync results
func (m *AppModel) viewSummary() string {
	if m.result == nil {
		return m.viewMainMenu()
	}

	result := m.result

	errColor := lipgloss.NewStyle().Foreground(Primary)
	if result.Errors > 0 {
		errColor = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true)
	}

	period := ""
	if result.Period != "" {
		period = fmt.Sprintf("\n  Periodo:       %s", result.Period)
	}

	inner := fmt.Sprintf(
		"  RESUMEN DE SINCRONIZACION\n\n"+
			"  Base de datos: %s\n"+
			"  Tabla:         %s\n"+
			"  Accion:        %s%s\n\n"+
			"  ─────────────────────────────\n"+
			"  Antes:         %s\n"+
			"  Insertados:    +%s\n"+
			"  Actualizados:  %s\n"+
			"  Omitidos:      %s\n"+
			"  Errores:       %s\n"+
			"  Despues:       %s\n"+
			"  ─────────────────────────────\n"+
			"  Duracion:      %s\n",
		result.Database,
		result.Table,
		getActionLabel(result.Action),
		period,
		formatNumber(result.RecordsBefore),
		formatNumber(result.Inserted),
		formatNumber(result.Updated),
		formatNumber(result.Skipped),
		errColor.Render(formatNumber(result.Errors)),
		formatNumber(result.RecordsAfter),
		formatDuration(result.Duration),
	)

	boxWidth := m.width - 8
	if boxWidth < 40 {
		boxWidth = 40
	}

	box := BoxStyle.Width(boxWidth).Render(inner)

	hint := HintStyle.Width(m.width - 4).Render("\n  Presiona cualquier tecla para continuar...")

	return "\n" + box + "\n" + hint
}

// viewStatus shows connection status
func (m *AppModel) viewStatus() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Width(m.width - 4).Render(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Estado de Conexiones

`))

	for name, dbCfg := range m.config.Databases {
		b.WriteString(fmt.Sprintf("  ► %s: %s:%d (%s)\n", name, dbCfg.Host, dbCfg.Port, dbCfg.Database))
	}

	b.WriteString(HintStyle.Width(m.width - 4).Render(`
  Presioná cualquier tecla para continuar...`))

	return b.String()
}

// viewInstalling shows installation progress
func (m *AppModel) viewInstalling() string {
	installDir := GetInstallDir()
	inPath := IsInPath(installDir)

	header := TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Instalar en el Sistema

  ► Directorio: %s
  ► En PATH: %s

  ¿Instalar? (s/n)

`, installDir, boolToYesNo(inPath)))

	footer := HintStyle.Width(m.width - 4).Render(`
  s/Sí · n/No · Esc volver`)

	return header + footer
}

// viewInstallDone shows installation result
func (m *AppModel) viewInstallDone() string {
	return TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ✅ Instalación Completada

  Presioná cualquier tecla para continuar...
`))
}

// viewConfig shows configuration
func (m *AppModel) viewConfig() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Width(m.width - 4).Render(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Configuración

`))

	if len(m.config.Databases) > 0 {
		b.WriteString("  Bases de datos:\n")
		for name, dbCfg := range m.config.Databases {
			b.WriteString(fmt.Sprintf("    ► %s → %s:%d/%s\n", name, dbCfg.Host, dbCfg.Port, dbCfg.Database))
		}
	}

	if m.config.Settings.DBFDirectories != nil && len(m.config.Settings.DBFDirectories) > 0 {
		b.WriteString("\n  Directorios .dbf:\n")
		for db, dir := range m.config.Settings.DBFDirectories {
			b.WriteString(fmt.Sprintf("    ► %s → %s\n", db, dir))
		}
	}

	b.WriteString(HintStyle.Width(m.width - 4).Render(`
  Presioná cualquier tecla para continuar...`))

	return b.String()
}

// viewQuit shows quit confirmation
func (m *AppModel) viewQuit() string {
	return TitleStyle.Width(m.width - 4).Render(fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ¿Confirmar salida? (s/n)

  s/Sí · n/No
`))
}

// Helper functions

func getActionLabel(action string) string {
	switch action {
	case "insert":
		return "Insertar nuevos registros"
	case "cobrador":
		return "Actualizar cobradores"
	case "full":
		return "Sync completo"
	case "back":
		return "Volver"
	case "quit":
		return "Salir"
	default:
		return action
	}
}

func boolToYesNo(b bool) string {
	if b {
		return "Sí"
	}
	return "No"
}

func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	result := ""
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		if count > 0 && count%3 == 0 {
			result = "," + result
		}
		result = string(s[i]) + result
		count++
	}
	return result
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", m, sec)
}

// MutedStyle creates a muted style
func MutedStyle(text string) string {
	return lipgloss.NewStyle().Foreground(Muted).Render(text)
}

// GetConfig returns the config for external access
func (m *AppModel) GetConfig() *config.Config {
	return m.config
}