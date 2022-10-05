package state_display

import (
	"time"
)

var OnStateDisplayChanged = func(stateDisplay StateDisplay) {}
var GlobalState = StateDisplay{}

var majorStepIndex = map[string]int{}
var minorStepIndex = map[string]map[string]int{}

type StateDisplay struct {
	MajorsSteps []MajorStep
}

type StepStatus = string

const (
	StepStatusPending = "PENDING"
	StepStatusDone    = "DONE"
)

type MajorStep struct {
	Name       string
	StartedAt  time.Time
	EndedAt    time.Time
	Status     StepStatus
	MinorSteps []MinorStep
}

type MinorStep struct {
	Name      string
	StartedAt time.Time
	EndedAt   time.Time
	Status    StepStatus
	LogLines  []string
}

func StartMajorStep(name string) {
	majorStepIndex[name] = len(GlobalState.MajorsSteps)
	GlobalState.MajorsSteps = append(GlobalState.MajorsSteps, MajorStep{
		Name:      name,
		StartedAt: time.Now(),
		Status:    StepStatusPending,
	})
	if OnStateDisplayChanged != nil {
		OnStateDisplayChanged(GlobalState)
	}
}

func EndMajorStep(name string) {
	majorStep := GlobalState.MajorsSteps[majorStepIndex[name]]
	majorStep.EndedAt = time.Now()
	majorStep.Status = StepStatusDone
	GlobalState.MajorsSteps[majorStepIndex[name]] = majorStep
	delete(majorStepIndex, name)

	if OnStateDisplayChanged != nil {
		OnStateDisplayChanged(GlobalState)
	}
}

func StartMinorStep(parentName string, name string) {
	majorStep := GlobalState.MajorsSteps[majorStepIndex[parentName]]
	if minorStepIndex[parentName] == nil {
		minorStepIndex[parentName] = map[string]int{}
	}
	minorStepIndex[parentName][name] = len(majorStep.MinorSteps)
	majorStep.MinorSteps = append(majorStep.MinorSteps, MinorStep{
		Name:      name,
		StartedAt: time.Now(),
		Status:    StepStatusPending,
	})
	GlobalState.MajorsSteps[majorStepIndex[parentName]] = majorStep

	if OnStateDisplayChanged != nil {
		OnStateDisplayChanged(GlobalState)
	}
}

func EndMinorStep(parentName string, name string) {
	majorStep := GlobalState.MajorsSteps[majorStepIndex[parentName]]
	minorStep := majorStep.MinorSteps[minorStepIndex[parentName][name]]
	minorStep.EndedAt = time.Now()
	minorStep.Status = StepStatusDone
	majorStep.MinorSteps[minorStepIndex[parentName][name]] = minorStep
	delete(minorStepIndex[parentName], name)
	GlobalState.MajorsSteps[majorStepIndex[parentName]] = majorStep

	if OnStateDisplayChanged != nil {
		OnStateDisplayChanged(GlobalState)
	}
}

func AddLogLine(parentName string, name string, line string) {
	majorStep := GlobalState.MajorsSteps[majorStepIndex[parentName]]
	minorStep := majorStep.MinorSteps[minorStepIndex[parentName][name]]
	minorStep.LogLines = append(minorStep.LogLines, line)
	majorStep.MinorSteps[minorStepIndex[parentName][name]] = minorStep
	GlobalState.MajorsSteps[majorStepIndex[parentName]] = majorStep

	if OnStateDisplayChanged != nil {
		OnStateDisplayChanged(GlobalState)
	}
}

func FindActiveMajorStepWithMinorStepNamed(name string) (parentName string) {
	for parentName, minorStepIndex := range minorStepIndex {
		if _, ok := minorStepIndex[name]; ok {
			return parentName
		}
	}
	return ""
}
