package logger

import (
	"barbe/core/state_display"
	"context"
	"github.com/gosuri/uilive"
	"github.com/rs/zerolog"
	"io"
	"strings"
	"sync"
	"time"
)

type LiveRunnerProgress struct {
	LogLevel zerolog.Level

	w     *uilive.Writer
	lines []io.Writer

	ctx       context.Context
	cancelCtx context.CancelFunc
	waitGroup sync.WaitGroup

	mu      sync.RWMutex
	display state_display.StateDisplay
}

func NewLiveRunnerProgress() *LiveRunnerProgress {
	ctx, cancel := context.WithCancel(context.Background())
	writer := uilive.New()
	//we need to control the flushing ourselves, so this essentially disables the auto-flushing
	writer.RefreshInterval = time.Hour
	return &LiveRunnerProgress{
		w:         writer,
		ctx:       ctx,
		cancelCtx: cancel,
		waitGroup: sync.WaitGroup{},
		mu:        sync.RWMutex{},
		display:   state_display.StateDisplay{},
	}
}

func (p *LiveRunnerProgress) Start() {
	p.w.Start()
	p.waitGroup.Add(1)
	go p.printWorker()
}

func (p *LiveRunnerProgress) UpdateState(display state_display.StateDisplay) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.display = display
}

func (p *LiveRunnerProgress) Close() {
	p.cancelCtx()
	p.waitGroup.Wait()
	p.printTasks()
}

func (p *LiveRunnerProgress) printWorker() {
	defer p.waitGroup.Done()
	ticker := time.NewTicker(300 * time.Millisecond)
	for {
		select {
		case <-p.ctx.Done():
			p.printTasks()
			return
		case <-ticker.C:
		}
		p.printTasks()
	}
}

func (p *LiveRunnerProgress) printTasks() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	defer p.w.Flush()

	msg := viewTasks(p.display, p.LogLevel)
	lines := strings.Split(msg, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if p.lines == nil || len(p.lines) <= i {
			p.lines = append(p.lines, p.w.Newline())
		}
		p.lines[i].Write([]byte(line + "\n"))
	}
	//print empty lines to clear the rest of the screen
	//for i := len(lines); i < len(p.lines); i++ {
	//	p.lines[i].Write([]byte("\n"))
	//}

}
