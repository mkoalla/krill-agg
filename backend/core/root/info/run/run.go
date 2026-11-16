package run

import "github.com/google/uuid"

// unique per process start. stamp it into anything discord keeps across
// restarts (e.g. button custom IDs) to tell a previous run's leftovers apart
var ID = uuid.NewString()
