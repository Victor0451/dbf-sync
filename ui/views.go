package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"dbf-sync/config"
)

// ── Layout helpers ────────────────────────────────────────────────────────────

// sep returns a full-width separator line using the current terminal width.
func (m *AppModel) sep() string {
	w := m.width - 4
	if w < 10 {
		w = 10
	}
	return strings.Repeat("─", w)
}

// pageTitle renders the top section: separator, title, optional breadcrumbs, nav hint.
func (m *AppModel) pageTitle(title string, crumbs []string, hint string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(Primary)
	hintStyle := lipgloss.NewStyle().Foreground(Muted).Italic(true)
	crumbStyle := lipgloss.NewStyle().Foreground(Accent)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  "+title) + "\n")
	if len(crumbs) > 0 {
		b.WriteString("\n")
		for _, c := range crumbs {
			b.WriteString(crumbStyle.Render("  › ") + c + "\n")
		}
	}
	b.WriteString("\n" + hintStyle.Render("  "+hint) + "\n")
	b.WriteString(m.sep() + "\n")
	return b.String()
}

// footer renders a bottom hint line.
func (m *AppModel) footer(hint string) string {
	return "\n" + HintStyle.Width(m.width-4).Render("  "+hint)
}

// errLine renders an inline error message.
func errLine(msg string) string {
	return ErrorStyle.Render("  " + msg) + "\n"
}

// ── View dispatcher ───────────────────────────────────────────────────────────

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
	case StateConfigForm:
		content = m.viewConfigForm()
	case StateSelectProfile:
		content = m.viewSelectProfile()
	case StateQuit:
		content = m.viewQuit()
	default:
		content = m.viewMainMenu()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// ── Individual views ──────────────────────────────────────────────────────────

func (m *AppModel) viewMainMenu() string {
	if m.list.Items() == nil || len(m.list.Items()) == 0 {
		m.loadMainMenu()
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Primary).
		Padding(0, 4).
		Width(m.width - 6).
		Align(lipgloss.Center)

	versionStyle := lipgloss.NewStyle().Foreground(Muted)
	poweredStyle := lipgloss.NewStyle().Foreground(Accent)

	// Show active profile in banner if set
	activeProfile := m.config.Settings.ActiveProfile
	var title string
	if activeProfile != "" {
		title = fmt.Sprintf("DBF-SYNC  [%s]", activeProfile)
	} else {
		title = "DBF-SYNC"
	}

	left := fmt.Sprintf("%s  %s", title, versionStyle.Render("v"+m.version))
	right := poweredStyle.Render("Powered by VML PROGRAMMING")
	pad := m.width - 20 - len(m.version) - len(title) - lipgloss.Width(right)
	if pad < 2 {
		pad = 2
	}
	subtitle := left + strings.Repeat(" ", pad) + right

	banner := titleStyle.Render(title) + "\n" + subtitle

	sep := lipgloss.NewStyle().Foreground(Muted).Render(m.sep())
	hint := HintStyle.Render("  Que queres hacer?")
	menu := m.list.View()
	nav := HintStyle.Render("  ↑/↓  navegar    Enter  confirmar    q  salir    p  perfiles")

	return banner + "\n" + sep + "\n" + hint + "\n\n" + menu + "\n" + nav
}

func (m *AppModel) viewSelectDB() string {
	header := m.pageTitle("Seleccionar Base de Datos", nil, "↑/↓  navegar    Enter  confirmar    Esc  volver")
	return header + m.list.View()
}

func (m *AppModel) viewSelectTable() string {
	if m.loading {
		return m.pageTitle(fmt.Sprintf("Conectando a %s...", m.db), nil, "") +
			"\n  " + m.spinner.View() + "  " + m.loadingMsg + "\n"
	}
	if m.err != nil {
		return m.pageTitle("Error de conexion", nil, "Presiona cualquier tecla para volver...") +
			errLine(m.err.Error())
	}
	hint := "↑/↓  navegar    Enter  confirmar    Esc  volver    [cfg] = tiene config"
	header := m.pageTitle(fmt.Sprintf("Seleccionar Tabla  [%s]", m.db), nil, hint)

	accentStyle := lipgloss.NewStyle().Foreground(Accent).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(Muted)
	var searchLine string
	if m.tableSearch != "" {
		searchLine = "  " + mutedStyle.Render("Filtro: ") + accentStyle.Render(m.tableSearch) +
			mutedStyle.Render("_") + "\n\n"
	} else {
		searchLine = "  " + mutedStyle.Render("Escribi para filtrar...") + "\n\n"
	}

	return header + searchLine + m.list.View()
}

func (m *AppModel) viewSelectAction() string {
	header := m.pageTitle(
		"Seleccionar Accion",
		[]string{
			fmt.Sprintf("Base de datos: %s", m.db),
			fmt.Sprintf("Tabla:         %s", m.table),
		},
		"↑/↓  navegar    Enter  confirmar    Esc  volver",
	)
	errLine := ""
	if m.err != nil {
		errLine = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("  Error: "+m.err.Error()) + "\n"
	}
	return header + errLine + m.list.View()
}

func (m *AppModel) viewBrowseFile() string {
	accentStyle := lipgloss.NewStyle().Foreground(Accent).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(Muted)

	header := m.pageTitle(
		"Seleccionar Archivo .dbf",
		[]string{
			fmt.Sprintf("Base de datos: %s", m.db),
			fmt.Sprintf("Tabla:         %s", m.table),
			fmt.Sprintf("Accion:        %s", getActionLabel(m.action)),
		},
		"↑/↓  navegar    Enter  abrir/entrar    ..  subir directorio    t  ruta manual    Esc  volver",
	)

	var b strings.Builder
	b.WriteString(header)
	b.WriteString(mutedStyle.Render("  " + m.currentDir) + "\n\n")

	if len(m.dirEntries) == 0 {
		b.WriteString(HintStyle.Render("  No hay archivos .dbf en este directorio\n"))
	} else {
		for i, entry := range m.dirEntries {
			icon := "  "
			if entry.IsDir {
				icon = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("D ")
			} else {
				icon = lipgloss.NewStyle().Foreground(Primary).Render("F ")
			}

			name := entry.Name
			size := ""
			if !entry.IsDir {
				size = mutedStyle.Render("  " + FormatSize(entry.Size))
			}

			if i == m.cursor {
				b.WriteString(accentStyle.Render("  ► ") + icon + accentStyle.Render(name) + size + "\n")
			} else {
				b.WriteString("    " + icon + name + size + "\n")
			}
		}
	}

	return b.String()
}

func (m *AppModel) viewManualPath() string {
	header := m.pageTitle(
		"Ingresar Ruta Manualmente",
		[]string{
			fmt.Sprintf("Base de datos: %s", m.db),
			fmt.Sprintf("Tabla:         %s", m.table),
		},
		"Enter  confirmar    Esc  cancelar",
	)
	return header + "\n  Ruta: " + m.textInput.View() + "\n"
}

func (m *AppModel) viewInputMonth() string {
	var errMsg string
	if m.err != nil {
		errMsg = errLine(m.err.Error())
	}
	header := m.pageTitle(
		"Actualizar Cobradores — Mes",
		[]string{
			fmt.Sprintf("Base de datos: %s", m.db),
			fmt.Sprintf("Tabla:         %s", m.table),
			fmt.Sprintf("Archivo:       %s", m.dbfPath),
		},
		"Ingresa el mes (1-12)    Enter  confirmar    Esc  volver",
	)
	return header + "\n  Mes: " + m.textInput.View() + "\n" + errMsg
}

func (m *AppModel) viewInputYear() string {
	var errMsg string
	if m.err != nil {
		errMsg = errLine(m.err.Error())
	}
	header := m.pageTitle(
		"Actualizar Cobradores — Ano",
		[]string{
			fmt.Sprintf("Base de datos: %s", m.db),
			fmt.Sprintf("Tabla:         %s", m.table),
			fmt.Sprintf("Mes:           %02d", m.month),
		},
		"Ingresa el ano (ej: 2026)    Enter  confirmar    Esc  volver",
	)
	return header + "\n  Ano: " + m.textInput.View() + "\n" + errMsg
}

func (m *AppModel) viewConfirm() string {
	crumbs := []string{
		fmt.Sprintf("Base de datos: %s", m.db),
		fmt.Sprintf("Tabla:         %s", m.table),
		fmt.Sprintf("Archivo:       %s", m.dbfPath),
		fmt.Sprintf("Accion:        %s", getActionLabel(m.action)),
	}
	if m.action == "cobrador" {
		crumbs = append(crumbs, fmt.Sprintf("Periodo:       %02d/%d", m.month, m.year))
	}

	header := m.pageTitle("Confirmar Sincronizacion", crumbs, "s  confirmar    n / Esc  cancelar")
	return header + "\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true).Render("  Confirmar? (s/n)") +
		"\n"
}

func (m *AppModel) viewProcessing() string {
	period := ""
	if m.action == "cobrador" && m.month > 0 {
		period = fmt.Sprintf("\n  Periodo:        %02d/%d", m.month, m.year)
	}

	spinnerStr := m.spinner.View()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(Primary)
	crumbStyle := lipgloss.NewStyle().Foreground(Accent)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render(fmt.Sprintf("  %s  Sincronizando...", spinnerStr)) + "\n\n")
	b.WriteString(crumbStyle.Render("  › ") + fmt.Sprintf("Base de datos:  %s\n", m.db))
	b.WriteString(crumbStyle.Render("  › ") + fmt.Sprintf("Tabla:          %s\n", m.table))
	b.WriteString(crumbStyle.Render("  › ") + fmt.Sprintf("Accion:         %s%s\n", getActionLabel(m.action), period))
	b.WriteString("\n")

	// Render progress bar
	progressBarView := m.progressModel.View()
	barStyle := lipgloss.NewStyle().Foreground(Accent)
	b.WriteString(barStyle.Render("  "+progressBarView) + "\n")

	// Render stats: "234,500 / 700,000", "ETA: 2m 15s", "34,200 rec/s"
	statsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	etaStyle := lipgloss.NewStyle().Foreground(Accent)
	speedStyle := lipgloss.NewStyle().Foreground(Primary)

	// Progress stats line
	currentStr := formatNumber(m.progressModel.LastCurrent)
	totalStr := "..."
	if m.progressModel.Total > 0 {
		totalStr = formatNumber(m.progressModel.Total)
	}
	statsLine := fmt.Sprintf("  %s / %s", currentStr, totalStr)

	// ETA line
	etaLine := ""
	if m.progressModel.ETA != "" {
		etaLine = "  ETA: " + m.progressModel.ETA
	}

	// Speed line
	speedLine := ""
	if m.progressModel.Speed != "" {
		speedLine = "  " + m.progressModel.Speed
	}

	if m.progressModel.Phase != "" {
		phaseStyle := lipgloss.NewStyle().Foreground(Accent).Bold(true)
		b.WriteString(phaseStyle.Render("  Fase: "+m.progressModel.Phase) + "\n")
	}
	b.WriteString(statsStyle.Render(statsLine) + "\n")
	if etaLine != "" {
		b.WriteString(etaStyle.Render(etaLine) + "\n")
	}
	if speedLine != "" {
		b.WriteString(speedStyle.Render(speedLine) + "\n")
	}

	// Error count banner - inline display during processing
	if m.errorCount > 0 {
		b.WriteString("\n")
		errBannerStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B"))
		b.WriteString(errBannerStyle.Render(fmt.Sprintf("  %d errores encontrados — se mostrarán al finalizar", m.errorCount)) + "\n")
	}

	b.WriteString("\n" + HintStyle.Render("  Este proceso puede tardar varios segundos.") + "\n")
	b.WriteString(m.sep() + "\n")
	return b.String()
}

func (m *AppModel) viewSummary() string {
	if m.result == nil {
		return m.viewMainMenu()
	}

	result := m.result
	errStyle := lipgloss.NewStyle().Foreground(Primary)
	if result.Errors > 0 {
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true)
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
			"  %s\n"+
			"  Antes:         %s\n"+
			"  Insertados:    +%s\n"+
			"  Actualizados:  %s\n"+
			"  Omitidos:      %s\n"+
			"  Duplicados:    %s\n"+
			"  Errores:       %s\n"+
			"  Despues:       %s\n"+
			"  %s\n"+
			"  Duracion:      %s\n",
		result.Database,
		result.Table,
		getActionLabel(result.Action),
		period,
		strings.Repeat("─", 30),
		formatNumber(result.RecordsBefore),
		formatNumber(result.Inserted),
		formatNumber(result.Updated),
		formatNumber(result.Skipped),
		formatNumber(result.Duplicates),
		errStyle.Render(formatNumber(result.Errors)),
		formatNumber(result.RecordsAfter),
		strings.Repeat("─", 30),
		formatDuration(result.Duration),
	)

	boxWidth := m.width - 8
	if boxWidth < 40 {
		boxWidth = 40
	}

	box := BoxStyle.Width(boxWidth).Render(inner)

	// Error panel - scrollable list of detailed sync errors
	errPanel := ""
	if len(result.SyncErrors) > 0 {
		var ep strings.Builder
		ep.WriteString("\n")
		ep.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFA500")).Render("  ▼ ERRORES DETALLADOS") + "\n")

		// Calculate visible range
		maxVisible := 5 // max errors to show without scrolling
		errors := result.SyncErrors
		offset := m.errorScrollOffset
		if offset < 0 {
			offset = 0
		}
		if offset >= len(errors) {
			offset = len(errors) - 1
			if offset < 0 {
				offset = 0
			}
		}

		end := offset + maxVisible
		if end > len(errors) {
			end = len(errors)
		}

		for i := offset; i < end; i++ {
			e := errors[i]
			// Category icon
			var icon string
			switch e.Category {
			case "connection":
				icon = "🔌"
			case "data":
				icon = "💾"
			case "validation":
				icon = "✅"
			case "timeout":
				icon = "⏱️"
			default:
				icon = "⚠️"
			}

			// Truncate message if too long
			msg := e.Message
			if len(msg) > 50 {
				msg = msg[:47] + "..."
			}

			// Format timestamp
			ts := e.Timestamp.Format("15:04:05")

			// Highlight current selection
			if i == offset {
				ep.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true).Render(fmt.Sprintf("  %s %s  [%s]", icon, msg, ts)) + "\n")
			} else {
				ep.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC")).Render(fmt.Sprintf("  %s %s  [%s]", icon, msg, ts)) + "\n")
			}
		}

		// Scroll indicator
		if len(errors) > maxVisible {
			scrollInfo := fmt.Sprintf("  (%d-%d/%d) j/k para navegar", offset+1, end, len(errors))
			ep.WriteString(lipgloss.NewStyle().Foreground(Muted).Render(scrollInfo) + "\n")
		}

		errPanel = ep.String()
	}

	hint := HintStyle.Width(m.width - 4).Render("\n  Presiona cualquier tecla para continuar...")

	return "\n" + box + errPanel + "\n" + hint
}

func (m *AppModel) viewStatus() string {
	var b strings.Builder
	b.WriteString(m.pageTitle("Estado de Conexiones", nil, "Presiona cualquier tecla para volver..."))

	if m.loading {
		b.WriteString(fmt.Sprintf("\n  %s  Probando conexiones...\n", m.spinner.View()))
		return b.String()
	}

	okStyle := lipgloss.NewStyle().Foreground(Primary).Bold(true)
	failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true)

	for name, dbCfg := range m.config.Databases {
		status := HintStyle.Render("...")
		if m.statusResults != nil {
			r := m.statusResults[name]
			if r.ok {
				status = okStyle.Render("OK")
			} else {
				msg := "FALLO"
				if r.err != nil {
					msg = "FALLO: " + r.err.Error()
				}
				status = failStyle.Render(msg)
			}
		}
		b.WriteString(fmt.Sprintf("  %-14s  %s:%d / %-16s  %s\n",
			name, dbCfg.Host, dbCfg.Port, dbCfg.Database, status))
	}

	return b.String()
}

func (m *AppModel) viewInstalling() string {
	installDir := GetInstallDir()
	inPath := IsInPath(installDir)
	header := m.pageTitle(
		"Instalar en el Sistema",
		[]string{
			fmt.Sprintf("Directorio: %s", installDir),
			fmt.Sprintf("En PATH:    %s", boolToYesNo(inPath)),
		},
		"s  instalar    n / Esc  cancelar",
	)
	return header + "\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true).Render("  Instalar? (s/n)") +
		"\n"
}

func (m *AppModel) viewInstallDone() string {
	if m.loading {
		return m.pageTitle("Instalando...", nil, "") +
			fmt.Sprintf("\n  %s  %s\n", m.spinner.View(), m.loadingMsg)
	}

	header := m.pageTitle("Instalacion", nil, "Presiona cualquier tecla para continuar...")
	var b strings.Builder
	b.WriteString(header)
	if m.installErr != nil {
		b.WriteString(errLine(fmt.Sprintf("Error: %v", m.installErr)))
	} else {
		b.WriteString(SuccessStyle.Render("  Instalado correctamente") + "\n\n")
		if m.installResult != "" {
			for _, line := range strings.Split(m.installResult, "\n") {
				if line != "" {
					b.WriteString(HintStyle.Render("  "+line) + "\n")
				}
			}
		}
	}
	return b.String()
}

func (m *AppModel) viewConfig() string {
	hint := "↑/↓  navegar    Enter  editar    N  nueva    D  eliminar    Esc  volver"
	header := m.pageTitle("Configuración de Conexiones", nil, hint)
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")

	if len(m.configEntries) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(Muted).Render("  (sin conexiones — presioná N para agregar)") + "\n")
		return b.String()
	}

	cursorStyle := lipgloss.NewStyle().Foreground(Primary).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))

	for i, name := range m.configEntries {
		db := m.config.Databases[name]
		line := fmt.Sprintf("  %-16s  %s:%d  /  %s", name, db.Host, db.Port, db.Database)
		if i == m.configCursor {
			b.WriteString(cursorStyle.Render("▶ "+line[2:]) + "\n")
		} else {
			b.WriteString(normalStyle.Render(line) + "\n")
		}
	}
	return b.String()
}

func (m *AppModel) viewConfigForm() string {
	var title string
	if m.configEditingDB == "" {
		title = "Nueva Conexión"
	} else {
		title = "Editar: " + m.configEditingDB
	}
	hint := "Tab/↑↓  navegar campos    Enter  siguiente/guardar    Ctrl+S  guardar    Esc  cancelar"
	header := m.pageTitle(title, nil, hint)

	labels := []string{"Nombre interno", "Host / IP", "Puerto", "Usuario", "Contraseña", "Base de datos MySQL", "Directorio DBF"}
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")

	labelStyle := lipgloss.NewStyle().Foreground(Muted).Width(22)
	activeLabel := lipgloss.NewStyle().Foreground(Primary).Bold(true).Width(22)

	for i, f := range m.configFormFields {
		lbl := labelStyle.Render(labels[i])
		if i == m.configFormIdx {
			lbl = activeLabel.Render(labels[i])
		}
		b.WriteString("  " + lbl + f.View() + "\n")
	}

	if m.configSaveErr != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true)
		b.WriteString("\n  " + errStyle.Render("Error: "+m.configSaveErr.Error()) + "\n")
	}

	return b.String()
}

func (m *AppModel) viewSelectProfile() string {
	header := m.pageTitle("Seleccionar Perfil", nil, "↑/↓  navegar    Enter  seleccionar    Esc  volver")
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")

	profileNames := m.config.ProfileNames()
	activeProfile := m.config.Settings.ActiveProfile

	// First option: "(ninguno)" to clear active profile
	cursorStyle := lipgloss.NewStyle().Foreground(Primary).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	activeMarkerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Bold(true)

	// Option 0: (ninguno)
	if m.profileCursor == 0 {
		b.WriteString(cursorStyle.Render("▶ (ninguno)") + "\n")
	} else {
		b.WriteString(normalStyle.Render("  (ninguno)") + "\n")
	}

	// Profile options
	for i, name := range profileNames {
		idx := i + 1 // +1 because index 0 is "(ninguno)"
		line := "  " + name
		// Show active marker if this profile is currently active
		if activeProfile == name {
			line = activeMarkerStyle.Render("▶ " + name + "  [activo]")
		}
		if m.profileCursor == idx {
			if activeProfile == name {
				b.WriteString(activeMarkerStyle.Render("▶ "+name+"  [activo]") + "\n")
			} else {
				b.WriteString(cursorStyle.Render("▶ "+name) + "\n")
			}
		} else {
			if activeProfile == name {
				b.WriteString(activeMarkerStyle.Render("  "+name+"  [activo]") + "\n")
			} else {
				b.WriteString(normalStyle.Render(line) + "\n")
			}
		}
	}

	// Show hint about no profiles
	if len(profileNames) == 0 {
		b.WriteString("\n")
		b.WriteString(MutedStyle("  (sin perfiles definidos — agregalos en config.yaml)") + "\n")
	}

	return b.String()
}

func (m *AppModel) viewQuit() string {
	header := m.pageTitle("Salir", nil, "s  confirmar    n / Esc  cancelar")
	return header + "\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true).Render("  Confirmar salida? (s/n)") +
		"\n"
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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
		return "Si"
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
	min := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", min, sec)
}

// MutedStyle creates a muted style
func MutedStyle(text string) string {
	return lipgloss.NewStyle().Foreground(Muted).Render(text)
}

// GetConfig returns the config for external access
func (m *AppModel) GetConfig() *config.Config {
	return m.config
}
