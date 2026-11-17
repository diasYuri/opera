package ui

import "github.com/charmbracelet/harmonica"

// activityModel owns the animated activity trace and invalidates stale frames
// when a run starts or ends.
type activityModel struct {
	generation uint64
	progress   float64
	velocity   float64
	target     float64
}

func (a *activityModel) Start() {
	a.progress, a.velocity, a.target = 0, 0, 1
	a.generation++
}

func (a *activityModel) Stop() { a.generation++ }

func (a *activityModel) Frame(generation uint64) bool {
	if generation != a.generation {
		return false
	}
	a.progress, a.velocity = harmonica.NewSpring(1.0/20, 8, 1).Update(a.progress, a.velocity, a.target)
	if a.progress > .98 {
		a.target = 0
	}
	if a.progress < .02 {
		a.target = 1
	}
	return true
}

func (a activityModel) Position() int { return min(5, max(0, int(a.progress*5))) }
