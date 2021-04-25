package store

import {
	"github.com/gofrs/flock"
}

type fsm struct {
	dataFile string
	lock     *flock.Flock
}
