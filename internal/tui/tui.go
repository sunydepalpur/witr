package tui

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pranshuparmar/witr/internal/proc"
	"github.com/pranshuparmar/witr/internal/process"
	"github.com/pranshuparmar/witr/pkg/model"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type tickMsg time.Time

type modelState int

const (
	stateTCP modelState = iota
	stateUDP
	stateProcesses
)

type tuiModel struct {
	state            modelState
	table            table.Model
	filterInput      textinput.Model
	filtering        bool
	connections      []model.Connection
	processes        []model.ProcessSummary
	paused           bool
	currentUser      string
	showAllProcesses bool
	confirmingKill   bool
	killPID          int
	detailsPID       int
	detailsTree      string
	sortColumn       int
	sortAsc          bool
	message          string
	messageTime      time.Time
	err              error
	width            int
	height           int
}

func initialModel() tuiModel {
	ti := textinput.New()
	ti.Placeholder = "Filter..."
	ti.CharLimit = 50
	ti.Width = 30

	currUser, _ := user.Current()
	username := ""
	if currUser != nil {
		username = currUser.Username
	}

	m := tuiModel{
		state:            stateTCP,
		filterInput:      ti,
		currentUser:      username,
		showAllProcesses: false,
		sortAsc:          true,
	}
	m.initTable()
	return m
}

func (m *tuiModel) initTable() {
	var columns []table.Column
	switch m.state {
	case stateTCP, stateUDP:
		columns = []table.Column{
			{Title: "Proto", Width: 6},
			{Title: "Port", Width: 8},
			{Title: "Local Address", Width: 25},
			{Title: "Remote Address", Width: 25},
			{Title: "State", Width: 12},
			{Title: "PID", Width: 8},
			{Title: "User", Width: 12},
			{Title: "Process Chain", Width: 40},
		}
	case stateProcesses:
		columns = []table.Column{
			{Title: "PID", Width: 8},
			{Title: "User", Width: 12},
			{Title: "Process Tree", Width: 60},
		}
	}

	// Add sort indicator
	if m.sortColumn < len(columns) {
		indicator := " ↑"
		if !m.sortAsc {
			indicator = " ↓"
		}
		columns[m.sortColumn].Title += indicator
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(m.height-15),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(true)
	t.SetStyles(s)

	m.table = t
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(tick(), m.refreshData())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m tuiModel) refreshData() tea.Cmd {
	if m.paused {
		return nil
	}
	switch m.state {
	case stateTCP, stateUDP:
		return tea.Batch(
			func() tea.Msg {
				conns, err := proc.GetAllConnections()
				if err != nil {
					return err
				}
				return conns
			},
			func() tea.Msg {
				procs, err := proc.GetAllProcesses()
				if err != nil {
					return err
				}
				return procs
			},
		)
	case stateProcesses:
		return func() tea.Msg {
			procs, err := proc.GetAllProcesses()
			if err != nil {
				return err
			}
			return procs
		}
	}
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.confirmingKill {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				if m.killPID > 0 {
					_ = syscall.Kill(m.killPID, syscall.SIGTERM)
				}
				m.confirmingKill = false
				m.killPID = 0
				return m, m.refreshData()
			case "n", "N", "esc":
				m.confirmingKill = false
				m.killPID = 0
				return m, nil
			}
		}
		return m, nil
	}

	if m.filtering {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter", "esc":
				m.filtering = false
				m.filterInput.Blur()
				m.updateRows()
				return m, nil
			}
		}
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.updateRows()
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.state = stateTCP
			m.initTable()
			m.detailsPID = 0
			m.detailsTree = ""
			m.sortColumn = 0
			return m, m.refreshData()
		case "2":
			m.state = stateUDP
			m.initTable()
			m.detailsPID = 0
			m.detailsTree = ""
			m.sortColumn = 0
			return m, m.refreshData()
		case "3":
			m.state = stateProcesses
			m.initTable()
			m.detailsPID = 0
			m.detailsTree = ""
			m.sortColumn = 0
			return m, m.refreshData()
		case "up", "down", "j", "k", "pgup", "pgdown", "home", "end":
			m.detailsPID = 0
			m.detailsTree = ""
		case "p":
			m.paused = !m.paused
			return m, nil
		case "/":
			m.filtering = true
			m.filterInput.Focus()
			return m, nil
		case "a":
			m.showAllProcesses = !m.showAllProcesses
			m.updateRows()
			return m, nil
		case "s":
			m.sortColumn = (m.sortColumn + 1) % len(m.table.Columns())
			m.sortAsc = true
			m.initTable()
			m.updateRows()
			return m, nil
		case "r":
			m.sortAsc = !m.sortAsc
			m.initTable()
			m.updateRows()
			return m, nil
		case "S":
			m.saveSnapshot()
			return m, nil
		case "x":
			selected := m.table.SelectedRow()
			if len(selected) > 0 {
				pidIdx := 0
				if m.state == stateTCP || m.state == stateUDP {
					pidIdx = 5
				}
				pid, _ := strconv.Atoi(selected[pidIdx])
				if pid > 0 {
					m.confirmingKill = true
					m.killPID = pid
				}
			}
			return m, nil
		case "enter":
			selected := m.table.SelectedRow()
			if len(selected) > 0 {
				pidIdx := 0
				if m.state == stateTCP || m.state == stateUDP {
					pidIdx = 5
				}
				pid, _ := strconv.Atoi(selected[pidIdx])
				if pid > 0 {
					m.detailsPID = pid
					m.updateDetails()
				}
			}
			return m, nil
		}
	case tickMsg:
		return m, tea.Batch(tick(), m.refreshData())
	case []model.Connection:
		m.connections = msg
		m.updateRows()
	case []model.ProcessSummary:
		m.processes = msg
		m.updateRows()
	case error:
		m.err = msg
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetHeight(m.height - 15)
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *tuiModel) updateRows() {
	var rows []table.Row
	filterRaw := strings.ToLower(m.filterInput.Value())
	filterPrefix := ""
	filterValue := filterRaw

	if strings.Contains(filterRaw, ":") {
		parts := strings.SplitN(filterRaw, ":", 2)
		filterPrefix = parts[0]
		filterValue = parts[1]
	}

	switch m.state {
	case stateTCP, stateUDP:
		protoFilter := "TCP"
		if m.state == stateUDP {
			protoFilter = "UDP"
		}

		procMap := make(map[int]model.ProcessSummary)
		for _, p := range m.processes {
			procMap[p.PID] = p
		}

		for _, c := range m.connections {
			if !strings.Contains(strings.ToUpper(c.Protocol), protoFilter) {
				continue
			}

			// User filter
			if !m.showAllProcesses {
				if m.currentUser != "" {
					if p, ok := procMap[c.PID]; ok && p.User != m.currentUser {
						continue
					}
				}
				// Hide system processes by default
				if p, ok := procMap[c.PID]; ok && isSystemProcess(p.Command) {
					continue
				}
			}

			chain := c.Process
			currPID := c.PID
			depth := 0
			for depth < 5 {
				p, ok := procMap[currPID]
				if !ok || p.PPID == 0 || p.PPID == currPID {
					break
				}
				parent, ok := procMap[p.PPID]
				if !ok {
					break
				}
				chain = parent.Command + " -> " + chain
				currPID = p.PPID
				depth++
			}

			userName := "unknown"
			if p, ok := procMap[c.PID]; ok {
				userName = p.User
			}

			row := table.Row{
				c.Protocol,
				strconv.Itoa(c.LocalPort),
				fmt.Sprintf("%s:%d", c.LocalAddr, c.LocalPort),
				fmt.Sprintf("%s:%d", c.RemoteAddr, c.RemotePort),
				c.State,
				fmt.Sprintf("%d", c.PID),
				userName,
				chain,
			}

			if filterValue != "" {
				match := false
				switch filterPrefix {
				case "pid":
					match = strings.Contains(row[5], filterValue)
				case "port":
					match = strings.Contains(row[1], filterValue) || strings.Contains(row[2], ":"+filterValue) || strings.Contains(row[3], ":"+filterValue)
				case "proto":
					match = strings.Contains(strings.ToLower(row[0]), filterValue)
				case "cmd":
					match = strings.Contains(strings.ToLower(row[7]), filterValue)
				case "user":
					match = strings.Contains(strings.ToLower(row[6]), filterValue)
				case "":
					for _, f := range row {
						if strings.Contains(strings.ToLower(f), filterValue) {
							match = true
							break
						}
					}
				}
				if !match {
					continue
				}
			}

			rows = append(rows, row)
		}
	case stateProcesses:
		children := make(map[int][]int)
		procMap := make(map[int]model.ProcessSummary)
		for _, p := range m.processes {
			procMap[p.PID] = p
			children[p.PPID] = append(children[p.PPID], p.PID)
		}

		var renderTree func(int, string)
		renderTree = func(pid int, indent string) {
			p, ok := procMap[pid]
			if !ok {
				return
			}

			// User filter
			if !m.showAllProcesses {
				if m.currentUser != "" && p.User != m.currentUser {
					hasUserChild := false
					var checkChildren func(int)
					checkChildren = func(id int) {
						if hasUserChild {
							return
						}
						for _, kid := range children[id] {
							if kp, ok := procMap[kid]; ok && kp.User == m.currentUser {
								hasUserChild = true
								return
							}
							checkChildren(kid)
						}
					}
					checkChildren(pid)
					if !hasUserChild {
						return
					}
				}
				// Hide system processes by default
				if isSystemProcess(p.Command) {
					return
				}
			}

			row := table.Row{
				fmt.Sprintf("%d", p.PID),
				p.User,
				indent + p.Command,
			}

			match := true
			if filterValue != "" {
				match = false
				switch filterPrefix {
				case "pid":
					match = strings.Contains(row[0], filterValue)
				case "user":
					match = strings.Contains(strings.ToLower(row[1]), filterValue)
				case "cmd":
					match = strings.Contains(strings.ToLower(row[2]), filterValue)
				case "":
					for _, f := range row {
						if strings.Contains(strings.ToLower(f), filterValue) {
							match = true
							break
						}
					}
				}
			}

			if match {
				rows = append(rows, row)
			}

			kids := children[pid]
			sort.Ints(kids)
			for i, childPID := range kids {
				newIndent := indent + "  "
				if i == len(kids)-1 {
					newIndent = indent + "  └─ "
				} else {
					newIndent = indent + "  ├─ "
				}
				renderTree(childPID, newIndent)
			}
		}

		var roots []int
		for _, p := range m.processes {
			if _, hasParent := procMap[p.PPID]; !hasParent || p.PPID == 0 || p.PPID == 1 {
				roots = append(roots, p.PID)
			}
		}
		if _, hasOne := procMap[1]; hasOne {
			roots = []int{1}
		}

		sort.Ints(roots)
		for _, root := range roots {
			renderTree(root, "")
		}
	}

	// Sorting
	if len(rows) > 0 && m.sortColumn < len(m.table.Columns()) {
		sort.SliceStable(rows, func(i, j int) bool {
			valI := rows[i][m.sortColumn]
			valJ := rows[j][m.sortColumn]

			// Numeric sort for PID
			if (m.state == stateTCP || m.state == stateUDP) && m.sortColumn == 5 ||
				(m.state == stateProcesses && m.sortColumn == 0) {
				numI, _ := strconv.Atoi(valI)
				numJ, _ := strconv.Atoi(valJ)
				if m.sortAsc {
					return numI < numJ
				}
				return numI > numJ
			}

			// Numeric sort for Port column
			if (m.state == stateTCP || m.state == stateUDP) && m.sortColumn == 1 {
				numI, _ := strconv.Atoi(valI)
				numJ, _ := strconv.Atoi(valJ)
				if m.sortAsc {
					return numI < numJ
				}
				return numI > numJ
			}

			// Numeric sort for Ports in Address columns
			if (m.state == stateTCP || m.state == stateUDP) && (m.sortColumn == 2 || m.sortColumn == 3) {
				portI := extractPort(valI)
				portJ := extractPort(valJ)
				if portI != portJ {
					if m.sortAsc {
						return portI < portJ
					}
					return portI > portJ
				}
			}

			if m.sortAsc {
				return valI < valJ
			}
			return valI > valJ
		})
	}

	m.table.SetRows(rows)
}

func (m *tuiModel) updateDetails() {
	if m.detailsPID == 0 {
		return
	}

	ancestry, err := process.BuildAncestry(m.detailsPID)
	if err != nil {
		m.detailsTree = "Error: " + err.Error()
		return
	}

	var lines []string
	for i, p := range ancestry {
		indent := ""
		for j := 0; j < i; j++ {
			indent += "  "
		}
		if i > 0 {
			indent += "└─ "
		}
		lines = append(lines, fmt.Sprintf("%s%s (pid %d)", indent, p.Command, p.PID))
	}
	m.detailsTree = strings.Join(lines, "\n")
}

func isSystemProcess(path string) bool {
	systemDirs := []string{
		"/System/",
		"/usr/libexec/",
		"/usr/sbin/",
		"/sbin/",
	}
	for _, dir := range systemDirs {
		if strings.HasPrefix(path, dir) {
			return true
		}
	}
	// Also check common system process names if path is not absolute
	systemNames := []string{
		"rapportd",
		"identityservicesd",
		"apsd",
		"mDNSResponder",
		"trustd",
		"sharingd",
		"ControlCenter",
		"symptomsd",
		"airportd",
	}
	for _, name := range systemNames {
		if path == name {
			return true
		}
	}
	return false
}

func extractPort(addr string) int {
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		port, _ := strconv.Atoi(addr[idx+1:])
		return port
	}
	return 0
}

func (m *tuiModel) saveSnapshot() {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("witr_snapshot_%s.md", timestamp)

	var content strings.Builder
	content.WriteString("# witr Snapshot - " + time.Now().Format(time.RFC1123) + "\n\n")

	if m.detailsTree != "" {
		content.WriteString("## Process Details (PID " + fmt.Sprintf("%d", m.detailsPID) + ")\n")
		content.WriteString("```\n" + m.detailsTree + "\n```\n\n")

		// Export logs for the process on macOS
		out, err := exec.Command("log", "show", "--predicate", fmt.Sprintf("processID == %d", m.detailsPID), "--last", "1m", "--style", "compact").Output()
		if err == nil && len(out) > 0 {
			content.WriteString("## Recent Logs (Last 1m)\n")
			content.WriteString("```\n" + string(out) + "\n```\n")
		}
	} else {
		content.WriteString("## Current View (" + []string{"TCP", "UDP", "Processes"}[m.state] + ")\n\n")
		cols := m.table.Columns()
		for i, col := range cols {
			content.WriteString("| " + col.Title + " ")
			if i == len(cols)-1 {
				content.WriteString("|\n")
			}
		}
		for i := range cols {
			content.WriteString("| --- ")
			if i == len(cols)-1 {
				content.WriteString("|\n")
			}
		}
		for _, row := range m.table.Rows() {
			for _, cell := range row {
				content.WriteString("| " + cell + " ")
			}
			content.WriteString("|\n")
		}
	}

	err := os.WriteFile(filename, []byte(content.String()), 0644)
	if err != nil {
		m.message = "Error saving snapshot: " + err.Error()
	} else {
		m.message = "Snapshot saved to " + filename
	}
	m.messageTime = time.Now()
}

func (m tuiModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\nError: %v\n\nPress q to quit", m.err)
	}

	var b strings.Builder

	// Title
	title := "witr Interactive Mode"
	if m.paused {
		title += " (PAUSED)"
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("57")).Bold(true).Render(title) + "\n\n")

	// Tabs
	tabs := []string{"[1] TCP", "[2] UDP", "[3] Processes"}
	for i, t := range tabs {
		style := lipgloss.NewStyle().Padding(0, 1)
		if int(m.state) == i {
			style = style.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true)
		} else {
			style = style.Foreground(lipgloss.Color("240"))
		}
		b.WriteString(style.Render(t))
		b.WriteString(" ")
	}

	// Mode toggle
	mode := "User Processes"
	if m.showAllProcesses {
		mode = "All Processes"
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  Mode: [a] " + mode))

	// Sort info
	if m.sortColumn < len(m.table.Columns()) {
		colName := m.table.Columns()[m.sortColumn].Title
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf("  Sort: [s] %s", colName)))
	}
	b.WriteString("\n\n")

	// Filter
	if m.filtering {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("57")).Render(" / ") + m.filterInput.View() + "\n")
	} else if m.filterInput.Value() != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" Filter: "+m.filterInput.Value()) + "\n")
	} else {
		b.WriteString("\n")
	}

	// Table
	b.WriteString(baseStyle.Render(m.table.View()) + "\n")

	// Message (Snapshot feedback)
	if m.message != "" && time.Since(m.messageTime) < 3*time.Second {
		b.WriteString("\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Padding(0, 1).
			Render(" "+m.message+" ") + "\n")
	}

	// Confirmation
	if m.confirmingKill {
		prompt := fmt.Sprintf(" Are you sure you want to kill PID %d? [y/n] ", m.killPID)
		b.WriteString("\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("160")).
			Bold(true).
			Padding(0, 1).
			Render(prompt) + "\n")
	}

	// Details
	if m.detailsTree != "" && !m.confirmingKill {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("57")).Bold(true).Render(" Details: ") + "\n" + m.detailsTree + "\n")
	}

	// Help
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	help := "\n  q: quit • 1-3: tabs • /: filter • a: all/user • s: sort • r: reverse • S: snapshot • p: pause • x: kill • enter: details"
	if m.detailsPID != 0 {
		help += " • esc: close details"
	}
	b.WriteString(helpStyle.Render(help) + "\n")

	return b.String()
}

func Run() error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
