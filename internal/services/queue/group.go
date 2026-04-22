package queue

import (
	"github.com/carlj/godownload/internal/models"
)

// ManagerGroup wraps multiple queue managers and provides an aggregated view
// and control interface. It implements the management subset of Manager.
type ManagerGroup struct {
	managers []*Manager
}

func NewGroup(managers ...*Manager) *ManagerGroup {
	return &ManagerGroup{managers: managers}
}

func (g *ManagerGroup) Start() {
	for _, m := range g.managers {
		m.Start()
	}
}

func (g *ManagerGroup) Stop() {
	for _, m := range g.managers {
		m.Stop()
	}
}

func (g *ManagerGroup) Pause() {
	for _, m := range g.managers {
		m.Pause()
	}
}

func (g *ManagerGroup) Resume() {
	for _, m := range g.managers {
		m.Resume()
	}
}

func (g *ManagerGroup) IsPaused() bool {
	if len(g.managers) == 0 {
		return false
	}
	return g.managers[0].IsPaused()
}

func (g *ManagerGroup) ActiveDownloads() []ActiveDownload {
	var all []ActiveDownload
	for _, m := range g.managers {
		all = append(all, m.ActiveDownloads()...)
	}
	return all
}

func (g *ManagerGroup) SetStatusTracker(st statusReporter) {
	for _, m := range g.managers {
		m.SetStatusTracker(st)
	}
}

func (g *ManagerGroup) RegisterProcessor(queueType models.QueueType, p Processor) {
	for _, m := range g.managers {
		// Only register if the manager handles this type (or has no filter)
		if len(m.typeFilter) == 0 {
			m.RegisterProcessor(queueType, p)
			continue
		}
		for _, tf := range m.typeFilter {
			if tf == string(queueType) {
				m.RegisterProcessor(queueType, p)
				break
			}
		}
	}
}
