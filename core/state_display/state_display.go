package state_display

import (
	"sync"
	"time"
)

var GlobalState = StateDisplay{
	mutex:                 &sync.Mutex{},
	majorStepIndex:        map[string]int{},
	minorStepIndex:        map[string]map[string]int{},
	OnStateDisplayChanged: func(stateDisplay StateDisplay) {},
}

type StateDisplay struct {
	mutex          *sync.Mutex
	majorStepIndex map[string]int
	minorStepIndex map[string]map[string]int

	MajorsSteps           []MajorStep
	Logs                  []string
	OnStateDisplayChanged func(stateDisplay StateDisplay)
}

type StepStatus = string

const (
	StepStatusPending = "PENDING"
	StepStatusDone    = "DONE"
	StepStatusError   = "ERROR"
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
	MaxLogs   int
	LogLines  []string
}

func (s *StateDisplay) StartMajorStep(name string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.majorStepIndex[name] = len(s.MajorsSteps)
	s.MajorsSteps = append(s.MajorsSteps, MajorStep{
		Name:      name,
		StartedAt: time.Now(),
		Status:    StepStatusPending,
	})
	if s.OnStateDisplayChanged != nil {
		s.OnStateDisplayChanged(*s)
	}
}

func (s *StateDisplay) EndMajorStepWith(name string, withError bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	majorStep := s.MajorsSteps[s.majorStepIndex[name]]
	majorStep.EndedAt = time.Now()
	majorStep.Status = StepStatusDone
	if withError {
		majorStep.Status = StepStatusError
	}
	s.MajorsSteps[s.majorStepIndex[name]] = majorStep
	delete(s.majorStepIndex, name)

	if s.OnStateDisplayChanged != nil {
		s.OnStateDisplayChanged(*s)
	}
}

func (s *StateDisplay) EndMajorStep(name string) {
	s.EndMajorStepWith(name, false)
}

func (s *StateDisplay) StartMinorStep(parentName string, name string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	majorStep := s.MajorsSteps[s.majorStepIndex[parentName]]
	if s.minorStepIndex[parentName] == nil {
		s.minorStepIndex[parentName] = map[string]int{}
	}
	s.minorStepIndex[parentName][name] = len(majorStep.MinorSteps)
	majorStep.MinorSteps = append(majorStep.MinorSteps, MinorStep{
		Name:      name,
		StartedAt: time.Now(),
		Status:    StepStatusPending,
		MaxLogs:   10,
	})
	s.MajorsSteps[s.majorStepIndex[parentName]] = majorStep

	if s.OnStateDisplayChanged != nil {
		s.OnStateDisplayChanged(*s)
	}
}

func (s *StateDisplay) EndMinorStepWith(parentName string, name string, withError bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	majorStep := s.MajorsSteps[s.majorStepIndex[parentName]]
	minorStep := majorStep.MinorSteps[s.minorStepIndex[parentName][name]]
	minorStep.EndedAt = time.Now()
	minorStep.Status = StepStatusDone
	if withError {
		minorStep.Status = StepStatusError
	}
	majorStep.MinorSteps[s.minorStepIndex[parentName][name]] = minorStep
	delete(s.minorStepIndex[parentName], name)
	s.MajorsSteps[s.majorStepIndex[parentName]] = majorStep

	if s.OnStateDisplayChanged != nil {
		s.OnStateDisplayChanged(*s)
	}
}

func (s *StateDisplay) EndMinorStep(parentName string, name string) {
	s.EndMinorStepWith(parentName, name, false)
}

func (s *StateDisplay) AddTopLevelLogLine(line string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.Logs = append(s.Logs, line)

	if s.OnStateDisplayChanged != nil {
		s.OnStateDisplayChanged(*s)
	}
}

func (s *StateDisplay) AddLogLine(parentName string, name string, line string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	majorStep := s.MajorsSteps[s.majorStepIndex[parentName]]
	minorStep := majorStep.MinorSteps[s.minorStepIndex[parentName][name]]
	minorStep.LogLines = append(minorStep.LogLines, line)
	if len(minorStep.LogLines) >= minorStep.MaxLogs {
		minorStep.LogLines = minorStep.LogLines[1:]
	}

	majorStep.MinorSteps[s.minorStepIndex[parentName][name]] = minorStep
	s.MajorsSteps[s.majorStepIndex[parentName]] = majorStep

	if s.OnStateDisplayChanged != nil {
		s.OnStateDisplayChanged(*s)
	}
}

func (s *StateDisplay) FindActiveMajorStepWithMinorStepNamed(name string) (parentName string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for parentName, minorStepIndex := range s.minorStepIndex {
		if _, ok := minorStepIndex[name]; ok {
			return parentName
		}
	}
	return ""
}
