package logger

import (
	"barbe/core/state_display"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/stopwatch"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
	"strings"
	"time"
)

type FancyOutput struct{}

func NewFancyOutput() *FancyOutput {
	return &FancyOutput{}
}

func (f FancyOutput) Write(p []byte) (n int, err error) {
	var event struct {
		Message string `json:"message"`
	}
	d := json.NewDecoder(bytes.NewReader(p))
	if err := d.Decode(&event); err != nil {
		return 0, fmt.Errorf("cannot decode event: %s", err)
	}
	if program != nil {
		program.Send(AddLogsMsg{logs: []string{event.Message}})
	}

	return len(p), nil
}

const (
	shoulder = "├── "
	elbow    = "└── "
	body     = "│   "
	indent   = "	"
)

var (
	program *tea.Program

	majorTaskTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FAFAFA"))

	minorTaskTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FAFAFA"))

	doneStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	timeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3C3C3C")).
			Italic(true)

	logStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3C3C3C"))
)

type AddLogsMsg struct {
	logs []string
}
type SetStateDisplayMsg struct {
	stateDisplay state_display.StateDisplay
}

type Model struct {
	logLevel     zerolog.Level
	maxLogs      int
	logs         []string
	viewport     viewport.Model
	stopwatch    stopwatch.Model
	ready        bool
	stateDisplay state_display.StateDisplay
}

func (m Model) Init() tea.Cmd {
	return m.stopwatch.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	contentSet := false
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

	case AddLogsMsg:
		m.logs = append(m.logs, msg.logs...)
		if len(m.logs) >= m.maxLogs {
			m.logs = m.logs[len(m.logs)-m.maxLogs:]
		}

	case SetStateDisplayMsg:
		m.stateDisplay = msg.stateDisplay
		if m.ready {
			m.viewport.SetContent(viewTasks(m.stateDisplay, m.logLevel))
			m.viewport.GotoBottom()
			contentSet = true
		}

	case tea.WindowSizeMsg:
		m.maxLogs = msg.Height / 2
		heightOffset := m.maxLogs + 1
		if m.logLevel > zerolog.DebugLevel {
			heightOffset = 0
			m.maxLogs = 0
		}

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-heightOffset)
			m.viewport.SetContent(viewTasks(m.stateDisplay, m.logLevel))
			m.viewport.GotoBottom()
			contentSet = true
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - heightOffset
		}
	}

	if !contentSet {
		m.viewport.SetContent(viewTasks(m.stateDisplay, m.logLevel))
		m.viewport.GotoBottom()
	}

	var cmds []tea.Cmd

	v, cmd := m.viewport.Update(msg)
	m.viewport = v
	cmds = append(cmds, cmd)

	s, cmd := m.stopwatch.Update(msg)
	m.stopwatch = s
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	if m.logLevel > zerolog.DebugLevel {
		return m.viewport.View()
	}
	line := strings.Repeat("─", max(0, m.viewport.Width))
	logsView := viewLogs(m.logs)
	logsView += strings.Repeat("\n", max(0, m.maxLogs-len(m.logs)))
	return m.viewport.View() + "\n" + line + logsView
}

func viewLogs(logs []string) string {
	var result string
	for i, line := range logs {
		result += logStyle.Render(line)
		if i < len(logs)-1 {
			result += "\n"
		}
	}
	return result
}

func viewTasks(stateDisplay state_display.StateDisplay, logLevel zerolog.Level) string {
	var result string
	for _, major := range stateDisplay.MajorsSteps {
		result += viewMajorTask(major, logLevel)
	}
	return result
}

func viewMajorTask(major state_display.MajorStep, logLevel zerolog.Level) string {
	result := majorTaskTitleStyle.Render(major.Name) + " "
	if major.Status == state_display.StepStatusDone {
		result += doneStatusStyle.Render("[DONE]") + " " + timeStyle.Render("("+displayDuration(major.EndedAt.Sub(major.StartedAt))+")")
	} else {
		result += timeStyle.Render("(" + displayTimeSince(major.StartedAt) + ")")
	}

	result += "\n"
	for stepCount, minor := range major.MinorSteps {
		minorStepDisplay := viewMinorTask(minor, logLevel)
		if minorStepDisplay == "" {
			continue
		}
		if stepCount == len(major.MinorSteps)-1 {
			result += elbow
		} else {
			result += shoulder
		}
		lines := strings.Split(minorStepDisplay, "\n")
		for i, line := range lines {
			if line == "" {
				continue
			}
			if i == 0 {
				result += line + "\n"
			} else {
				filler := body
				if stepCount == len(major.MinorSteps)-1 {
					filler = indent
				}
				result += filler + line + "\n"
			}
		}
	}
	return result
}

func viewMinorTask(minor state_display.MinorStep, logLevel zerolog.Level) string {
	shouldDisplay := false
	if minor.Status == state_display.StepStatusDone {
		shouldDisplay = minor.EndedAt.Sub(minor.StartedAt) > 200*time.Millisecond
	} else {
		shouldDisplay = time.Since(minor.StartedAt) > 200*time.Millisecond
	}
	if logLevel > zerolog.DebugLevel && !shouldDisplay {
		return ""
	}
	result := minorTaskTitleStyle.Render(minor.Name) + " "
	if minor.Status == state_display.StepStatusDone {
		result += doneStatusStyle.Render("[DONE]") + " " + timeStyle.Render("("+displayDuration(minor.EndedAt.Sub(minor.StartedAt))+")")
	} else {
		result += timeStyle.Render("(" + displayTimeSince(minor.StartedAt) + ")")
	}
	if len(minor.LogLines) > 0 {
		result += "\n"
		for i, line := range minor.LogLines {
			result += logStyle.Render(body + line)
			if i != len(minor.LogLines)-1 {
				result += "\n"
			}
		}
	}
	return result
}

func displayTimeSince(t time.Time) string {
	return displayDuration(time.Since(t))
}

func displayDuration(t time.Duration) string {
	return fmt.Sprintf("%.1fs", t.Seconds())
}

func StartFancyDisplay(logger zerolog.Logger) {
	state_display.OnStateDisplayChanged = func(display state_display.StateDisplay) {
		if program == nil {
			return
		}
		program.Send(SetStateDisplayMsg{stateDisplay: display})
	}
	go func() {
		program = tea.NewProgram(Model{
			logLevel:     logger.GetLevel(),
			logs:         make([]string, 0),
			maxLogs:      10,
			stateDisplay: state_display.GlobalState,
			stopwatch:    stopwatch.NewWithInterval(time.Millisecond * 100),
		}, tea.WithAltScreen())
		if err := program.Start(); err != nil {
			logger.Debug().Err(err).Msg("error starting fancy display")
		}
	}()
}
