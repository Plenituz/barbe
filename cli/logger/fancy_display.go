package logger

import (
	"barbe/core/state_display"
	"bytes"
	"encoding/json"
	"fmt"
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
		Level   string `json:"level"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	d := json.NewDecoder(bytes.NewReader(p))
	if err := d.Decode(&event); err != nil {
		fmt.Println("error decoding event", err)
		return 0, fmt.Errorf("cannot decode event: %s", err)
	}
	style := logStyle
	text := string(p)

	if event.Message != "" {
		text = event.Message
	}
	if event.Error != "" {
		text = event.Error
	}
	switch l, _ := zerolog.ParseLevel(event.Level); l {
	case zerolog.WarnLevel:
		style = warnStyle
	case zerolog.ErrorLevel, zerolog.FatalLevel, zerolog.PanicLevel:
		style = errStyle
	}
	if len(text) > 300 {
		text = text[:300] + "..."
	}
	state_display.GlobalState.AddTopLevelLogLine(style.Render(text))
	return len(p), nil
}

const (
	shoulder = "├── "
	elbow    = "└── "
	body     = "│   "
	indent   = "    "
)

var (
	majorTaskTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FAFAFA"))

	minorTaskTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FAFAFA"))

	doneStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)
	errStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D50000")).
			Bold(true)

	timeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3C3C3C")).
			Italic(true)

	logStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3C3C3C"))
	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D50000"))
	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E2B603"))
)

func viewLogs(logs []string) string {
	var result string
	for i, line := range logs {
		result += line
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
	result += viewLogs(stateDisplay.Logs)
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
	if /*logLevel > zerolog.DebugLevel &&*/ !shouldDisplay {
		return ""
	}
	result := minorTaskTitleStyle.Render(minor.Name) + " "
	if minor.Status == state_display.StepStatusDone {
		result += doneStatusStyle.Render("[DONE]") + " " + timeStyle.Render("("+displayDuration(minor.EndedAt.Sub(minor.StartedAt))+")")
	} else if minor.Status == state_display.StepStatusDone {
		result += errStatusStyle.Render("[ERR]") + " " + timeStyle.Render("("+displayDuration(minor.EndedAt.Sub(minor.StartedAt))+")")
	} else {
		result += timeStyle.Render("(" + displayTimeSince(minor.StartedAt) + ")")
	}
	if minor.Status != state_display.StepStatusDone && len(minor.LogLines) > 0 {
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

//https://github.com/bvobart/mllint/blob/v0.7.2/commands/mllint/progress_live.go
func StartFancyDisplay(logger zerolog.Logger) func() {
	pg := NewLiveRunnerProgress()
	pg.LogLevel = logger.GetLevel()
	pg.Start()
	state_display.GlobalState.OnStateDisplayChanged = func(display state_display.StateDisplay) {
		pg.UpdateState(display)
	}
	return pg.Close
}
